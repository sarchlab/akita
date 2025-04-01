package tlb

import (
	"log"
	"reflect"
	"fmt"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/mem/mem"
	"github.com/sarchlab/akita/v4/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// Comp is a cache(TLB) that maintains some page information.
type Comp struct {
	*sim.TickingComponent
	sim.MiddlewareHolder

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	LowModule sim.Port

	numSets        int
	numWays        int
	pageSize       uint64
	numReqPerCycle int
	state          string

	Sets []internal.Set

	mshr                mshr
	respondingMSHREntry *mshrEntry

	isPaused bool
}

//Reset sets all the entries in the TLB to be invalid
func (c *Comp) reset() {
	c.Sets = make([]internal.Set, c.numSets)
	for i := 0; i < c.numSets; i++ {
		set := internal.NewSet(c.numWays)
		c.Sets[i] = set
	}
}

func (c *Comp) Tick() bool {
	return c.MiddlewareHolder.Tick()
}

type tlbMiddleware struct {
	*Comp
}

// Tick defines how TLB update states at each cycle
func (m *tlbMiddleware) Tick() bool {
	madeProgress := false
	madeProgress = m.performCtrlReq() || madeProgress

    fmt.Println("my state is ")

	switch m.state {
	    case "enable":
		default:
		    fmt.Println("my state is enable")
			for i := 0; i < m.numReqPerCycle; i++ {
				madeProgress = m.respondMSHREntry() || madeProgress
			}
			for i := 0; i < m.numReqPerCycle; i++ {
				madeProgress = m.lookup() || madeProgress
			}
			for i := 0; i < m.numReqPerCycle; i++ {
				madeProgress = m.parseBottom() || madeProgress
			}

		case "drain":
			fmt.Println("my state is drain")
			for i := 0; i < m.numReqPerCycle; i++ {
		        madeProgress = m.respondMSHREntry() || madeProgress
			}
			for i := 0; i < m.numReqPerCycle; i++ {
				madeProgress = m.parseBottom() || madeProgress
			}

            if m.mshr.IsEmpty() && m.bottomPort.PeekIncoming() == nil {
            	m.state = "pause"
            }

		case "pause":
			// No action
	}

	return madeProgress
}

func (m *tlbMiddleware) respondMSHREntry() bool {
	if m.respondingMSHREntry == nil {
		return false
	}

	mshrEntry := m.respondingMSHREntry
	page := mshrEntry.page
	req := mshrEntry.Requests[0]
	rspToTop := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	err := m.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		m.respondingMSHREntry = nil
	}

	tracing.TraceReqComplete(req, m.Comp)
	return true
}

func (m *tlbMiddleware) lookup() bool {
	msg := m.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	req := msg.(*vm.TranslationReq)

	mshrEntry := m.mshr.Query(req.PID, req.VAddr)
	if mshrEntry != nil {
		return m.processTLBMSHRHit(mshrEntry, req)
	}

	setID := m.vAddrToSetID(req.VAddr)
	set := m.Sets[setID]
	wayID, page, found := set.Lookup(req.PID, req.VAddr)
	if found && page.Valid {
		return m.handleTranslationHit(req, setID, wayID, page)
	}

	return m.handleTranslationMiss(req)
}

func (m *tlbMiddleware) handleTranslationHit(
	req *vm.TranslationReq,
	setID, wayID int,
	page vm.Page,
) bool {
    ok := m.sendRspToTop(req, page)
	if !ok {
		return false
	}

	m.visit(setID, wayID)
	m.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(req, m.Comp)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, m.Comp), m.Comp, "hit")
	tracing.TraceReqComplete(req, m.Comp)

	return true
}

func (m *tlbMiddleware) handleTranslationMiss(
	req *vm.TranslationReq,
) bool {
	if m.mshr.IsFull() {
		return false
	}

	fetched := m.fetchBottom(req)
	if fetched {
		m.topPort.RetrieveIncoming()
		tracing.TraceReqReceive(req, m.Comp)
		tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, m.Comp), m.Comp, "miss")
		return true
	}

	return false
}

func (m *tlbMiddleware) vAddrToSetID(vAddr uint64) (setID int) {
	return int(vAddr / m.pageSize % uint64(m.numSets))
}

func (m *tlbMiddleware) sendRspToTop(
	req *vm.TranslationReq,
	page vm.Page,
) bool {
	rsp := vm.TranslationRspBuilder{}.
		WithSrc(m.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()

	err := m.topPort.Send(rsp)

	return err == nil
}

func (m *tlbMiddleware) processTLBMSHRHit(
	mshrEntry *mshrEntry,
	req *vm.TranslationReq,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, req)

	m.topPort.RetrieveIncoming()
	tracing.TraceReqReceive(req, m.Comp)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, m.Comp), m.Comp, "mshr-hit")

	return true
}

func (m *tlbMiddleware) fetchBottom(req *vm.TranslationReq) bool {
	fetchBottom := vm.TranslationReqBuilder{}.
		WithSrc(m.bottomPort).
		WithDst(m.LowModule).
		WithPID(req.PID).
		WithVAddr(req.VAddr).
		WithDeviceID(req.DeviceID).
		Build()
	err := m.bottomPort.Send(fetchBottom)
	if err != nil {
		return false
	}

	mshrEntry := m.mshr.Add(req.PID, req.VAddr)
	mshrEntry.Requests = append(mshrEntry.Requests, req)
	mshrEntry.reqToBottom = fetchBottom

	tracing.TraceReqInitiate(fetchBottom, m.Comp,
		tracing.MsgIDAtReceiver(req, m.Comp))

	return true
}

func (m *tlbMiddleware) parseBottom() bool {
    fmt.Println("parseBottom() called")
	if m.respondingMSHREntry != nil {
	    fmt.Println("Already responding to an entry")
		return false
	}

	item := m.bottomPort.PeekIncoming()
	if item == nil {
	    fmt.Println("No incoming message at bottomPort")
		return false
	}

	rsp := item.(*vm.TranslationRsp)
	page := rsp.Page
	fmt.Println("parseBottom() received TranslationRsp with Page:", page)

	mshrEntryPresent := m.mshr.IsEntryPresent(rsp.Page.PID, rsp.Page.VAddr)
	if !mshrEntryPresent {
	    fmt.Println("MSHR entry not found for", rsp.Page.PID, rsp.Page.VAddr)
		m.bottomPort.RetrieveIncoming()
		return true
	}

	setID := m.vAddrToSetID(page.VAddr)
	set := m.Sets[setID]
	wayID, ok := m.Sets[setID].Evict()
	if !ok {
		panic("failed to evict")
	}
	set.Update(wayID, page)
	set.Visit(wayID)

	mshrEntry := m.mshr.GetEntry(rsp.Page.PID, rsp.Page.VAddr)
	m.respondingMSHREntry = mshrEntry
	mshrEntry.page = page

	m.mshr.Remove(rsp.Page.PID, rsp.Page.VAddr)
	m.bottomPort.RetrieveIncoming()
	tracing.TraceReqFinalize(mshrEntry.reqToBottom, m.Comp)

    fmt.Println("Final TLB state:", m.Sets)
	return true
}

func (m *tlbMiddleware) performCtrlReq() bool {
    fmt.Println("performCtrlReq() called")  // Debugging output
	item := m.controlPort.PeekIncoming()
	if item == nil {
	    fmt.Println("false1")  // Debugging output
		return false
	}
	fmt.Printf("Type of item: %T, value: %+v\n", item, item)
    fmt.Println("performCtrlReq() called")
	item = m.controlPort.RetrieveIncoming()
	fmt.Println(item)

	switch req := item.(type) {
	case *FlushReq:
	    fmt.Println("req is",req)
		return m.handleTLBFlush(req)
	case *RestartReq:
	    fmt.Println("req is",req)
		return m.handleTLBRestart(req)
    case *mem.ControlMsg:
        fmt.Println("yes")
    	if req.Enable {
    	    fmt.Println("yes")
    		m.state = "enable"
    	} else if req.Drain {
    		m.state = "drain"
    	} else if req.Pause {
    		m.state = "pause"
    	}
	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	}

	return true
}

func (m *tlbMiddleware) visit(setID, wayID int) {
	set := m.Sets[setID]
	set.Visit(wayID)
}

func (m *tlbMiddleware) handleTLBFlush(req *FlushReq) bool {
	rsp := FlushRspBuilder{}.
		WithSrc(m.controlPort).
		WithDst(req.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	for _, vAddr := range req.VAddr {
		setID := m.vAddrToSetID(vAddr)
		set := m.Sets[setID]
		wayID, page, found := set.Lookup(req.PID, vAddr)
		if !found {
			continue
		}

		page.Valid = false
		set.Update(wayID, page)
	}

	m.mshr.Reset()
	m.isPaused = true
	return true
}

func (m *tlbMiddleware) handleTLBRestart(req *RestartReq) bool {
	rsp := RestartRspBuilder{}.
		WithSrc(m.controlPort).
		WithDst(req.Src).
		Build()

	err := m.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	m.isPaused = false

	for m.topPort.RetrieveIncoming() != nil {
		m.topPort.RetrieveIncoming()
	}

	for m.bottomPort.RetrieveIncoming() != nil {
		m.bottomPort.RetrieveIncoming()
	}

	return true
}
