package tlb

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/mem/vm/tlb/internal"
	"github.com/sarchlab/akita/v4/sim"
	"github.com/sarchlab/akita/v4/tracing"
)

// Comp is a cache(TLB) that maintains some page information.
type Comp struct {
	*sim.TickingComponent

	topPort     sim.Port
	bottomPort  sim.Port
	controlPort sim.Port

	LowModule sim.Port

	numSets        int
	numWays        int
	pageSize       uint64
	numReqPerCycle int

	Sets []internal.Set

	mshr                mshr
	respondingMSHREntry *mshrEntry

	isPaused bool
}

// Reset sets all the entries int he TLB to be invalid
func (c *Comp) reset() {
	c.Sets = make([]internal.Set, c.numSets)
	for i := 0; i < c.numSets; i++ {
		set := internal.NewSet(c.numWays)
		c.Sets[i] = set
	}
}

// Tick defines how TLB update states at each cycle
func (c *Comp) Tick() bool {
	madeProgress := false

	madeProgress = c.performCtrlReq() || madeProgress

	if !c.isPaused {
		for i := 0; i < c.numReqPerCycle; i++ {
			madeProgress = c.respondMSHREntry() || madeProgress
		}

		for i := 0; i < c.numReqPerCycle; i++ {
			madeProgress = c.lookup() || madeProgress
		}

		for i := 0; i < c.numReqPerCycle; i++ {
			madeProgress = c.parseBottom() || madeProgress
		}
	}

	return madeProgress
}

func (c *Comp) respondMSHREntry() bool {
	if c.respondingMSHREntry == nil {
		return false
	}

	mshrEntry := c.respondingMSHREntry
	page := mshrEntry.page
	req := mshrEntry.Requests[0]
	rspToTop := vm.TranslationRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()
	err := c.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	mshrEntry.Requests = mshrEntry.Requests[1:]
	if len(mshrEntry.Requests) == 0 {
		c.respondingMSHREntry = nil
	}

	tracing.TraceReqComplete(req, c)
	return true
}

func (c *Comp) lookup() bool {
	msg := c.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	req := msg.(*vm.TranslationReq)

	mshrEntry := c.mshr.Query(req.PID, req.VAddr)
	if mshrEntry != nil {
		return c.processTLBMSHRHit(mshrEntry, req)
	}

	setID := c.vAddrToSetID(req.VAddr)
	set := c.Sets[setID]
	wayID, page, found := set.Lookup(req.PID, req.VAddr)
	if found && page.Valid {
		return c.handleTranslationHit(req, setID, wayID, page)
	}

	return c.handleTranslationMiss(req)
}

func (c *Comp) handleTranslationHit(
	req *vm.TranslationReq,
	setID, wayID int,
	page vm.Page,
) bool {
	ok := c.sendRspToTop(req, page)
	if !ok {
		return false
	}

	c.visit(setID, wayID)
	c.topPort.RetrieveIncoming()

	tracing.TraceReqReceive(req, c)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, c), c, "hit")
	tracing.TraceReqComplete(req, c)

	return true
}

func (c *Comp) handleTranslationMiss(
	req *vm.TranslationReq,
) bool {
	if c.mshr.IsFull() {
		return false
	}

	fetched := c.fetchBottom(req)
	if fetched {
		c.topPort.RetrieveIncoming()
		tracing.TraceReqReceive(req, c)
		tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, c), c, "miss")
		return true
	}

	return false
}

func (c *Comp) vAddrToSetID(vAddr uint64) (setID int) {
	return int(vAddr / c.pageSize % uint64(c.numSets))
}

func (c *Comp) sendRspToTop(
	req *vm.TranslationReq,
	page vm.Page,
) bool {
	rsp := vm.TranslationRspBuilder{}.
		WithSrc(c.topPort).
		WithDst(req.Src).
		WithRspTo(req.ID).
		WithPage(page).
		Build()

	err := c.topPort.Send(rsp)

	return err == nil
}

func (c *Comp) processTLBMSHRHit(
	mshrEntry *mshrEntry,
	req *vm.TranslationReq,
) bool {
	mshrEntry.Requests = append(mshrEntry.Requests, req)

	c.topPort.RetrieveIncoming()
	tracing.TraceReqReceive(req, c)
	tracing.AddTaskStep(tracing.MsgIDAtReceiver(req, c), c, "mshr-hit")

	return true
}

func (c *Comp) fetchBottom(req *vm.TranslationReq) bool {
	fetchBottom := vm.TranslationReqBuilder{}.
		WithSrc(c.bottomPort).
		WithDst(c.LowModule).
		WithPID(req.PID).
		WithVAddr(req.VAddr).
		WithDeviceID(req.DeviceID).
		Build()
	err := c.bottomPort.Send(fetchBottom)
	if err != nil {
		return false
	}

	mshrEntry := c.mshr.Add(req.PID, req.VAddr)
	mshrEntry.Requests = append(mshrEntry.Requests, req)
	mshrEntry.reqToBottom = fetchBottom

	tracing.TraceReqInitiate(fetchBottom, c,
		tracing.MsgIDAtReceiver(req, c))

	return true
}

func (c *Comp) parseBottom() bool {
	if c.respondingMSHREntry != nil {
		return false
	}

	item := c.bottomPort.PeekIncoming()
	if item == nil {
		return false
	}

	rsp := item.(*vm.TranslationRsp)
	page := rsp.Page

	mshrEntryPresent := c.mshr.IsEntryPresent(rsp.Page.PID, rsp.Page.VAddr)
	if !mshrEntryPresent {
		c.bottomPort.RetrieveIncoming()
		return true
	}

	setID := c.vAddrToSetID(page.VAddr)
	set := c.Sets[setID]
	wayID, ok := c.Sets[setID].Evict()
	if !ok {
		panic("failed to evict")
	}
	set.Update(wayID, page)
	set.Visit(wayID)

	mshrEntry := c.mshr.GetEntry(rsp.Page.PID, rsp.Page.VAddr)
	c.respondingMSHREntry = mshrEntry
	mshrEntry.page = page

	c.mshr.Remove(rsp.Page.PID, rsp.Page.VAddr)
	c.bottomPort.RetrieveIncoming()
	tracing.TraceReqFinalize(mshrEntry.reqToBottom, c)

	return true
}

func (c *Comp) performCtrlReq() bool {
	item := c.controlPort.PeekIncoming()
	if item == nil {
		return false
	}

	item = c.controlPort.RetrieveIncoming()

	switch req := item.(type) {
	case *FlushReq:
		return c.handleTLBFlush(req)
	case *RestartReq:
		return c.handleTLBRestart(req)
	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(req))
	}

	return true
}

func (c *Comp) visit(setID, wayID int) {
	set := c.Sets[setID]
	set.Visit(wayID)
}

func (c *Comp) handleTLBFlush(req *FlushReq) bool {
	rsp := FlushRspBuilder{}.
		WithSrc(c.controlPort).
		WithDst(req.Src).
		Build()

	err := c.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	for _, vAddr := range req.VAddr {
		setID := c.vAddrToSetID(vAddr)
		set := c.Sets[setID]
		wayID, page, found := set.Lookup(req.PID, vAddr)
		if !found {
			continue
		}

		page.Valid = false
		set.Update(wayID, page)
	}

	c.mshr.Reset()
	c.isPaused = true
	return true
}

func (c *Comp) handleTLBRestart(req *RestartReq) bool {
	rsp := RestartRspBuilder{}.
		WithSrc(c.controlPort).
		WithDst(req.Src).
		Build()

	err := c.controlPort.Send(rsp)
	if err != nil {
		return false
	}

	c.isPaused = false

	for c.topPort.RetrieveIncoming() != nil {
		c.topPort.RetrieveIncoming()
	}

	for c.bottomPort.RetrieveIncoming() != nil {
		c.bottomPort.RetrieveIncoming()
	}

	return true
}
