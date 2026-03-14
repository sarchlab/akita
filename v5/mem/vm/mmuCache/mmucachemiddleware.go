package mmuCache

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/tlb"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

type mmuCacheMiddleware struct {
	comp *modeling.Component[Spec, State]
}

func (m *mmuCacheMiddleware) topPort() sim.Port {
	return m.comp.GetPortByName("Top")
}

func (m *mmuCacheMiddleware) bottomPort() sim.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *mmuCacheMiddleware) controlPort() sim.Port {
	return m.comp.GetPortByName("Control")
}

func (m *mmuCacheMiddleware) Tick() bool {
	madeProgress := false
	next := m.comp.GetNextState()

	switch next.CurrentState {
	case mmuCacheStateDrain:
		madeProgress = m.handleDrain() || madeProgress
	case mmuCacheStatePause:
		return false
	case mmuCacheStateFlush:
		madeProgress = m.handleFlush() || madeProgress
	default:
		madeProgress = m.handleEnable() || madeProgress
	}
	return madeProgress
}

func (m *mmuCacheMiddleware) handleDrain() bool {
	madeProgress := m.processRequests()

	if m.bottomPort().PeekIncoming() == nil && m.topPort().PeekIncoming() == nil {
		next := m.comp.GetNextState()
		next.CurrentState = mmuCacheStatePause
		tracing.AddMilestone(
			m.comp.Name()+".drain",
			tracing.MilestoneKindHardwareResource,
			m.comp.Name()+".",
			m.comp.Name(),
			m.comp,
		)
	}

	return madeProgress
}

func (m *mmuCacheMiddleware) handleFlush() bool {
	next := m.comp.GetNextState()
	if !next.InflightFlushReqActive {
		return false
	}

	if m.topPort().PeekIncoming() == nil && m.bottomPort().PeekIncoming() == nil {
		return m.processMMUCacheFlush()
	}

	return m.processRequests()
}

// handleEnable processes requests when cache is in enabled state.
func (m *mmuCacheMiddleware) handleEnable() bool {
	return m.processRequests()
}

// processRequests handles both incoming lookup requests and bottom port responses.
func (m *mmuCacheMiddleware) processRequests() bool {
	madeProgress := false
	spec := m.comp.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.lookup() || madeProgress
	}
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.handleBottomPort() || madeProgress
	}
	return madeProgress
}

func (m *mmuCacheMiddleware) lookup() bool {
	if !m.bottomPort().CanSend() {
		return false
	}

	msgI := m.topPort().PeekIncoming()
	if msgI == nil {
		return false
	}

	msg, ok := msgI.(*vm.TranslationReq)
	if !ok || msg == nil {
		return false
	}

	return m.walkCacheLevels(msg)
}

func (m *mmuCacheMiddleware) walkCacheLevels(
	msg *vm.TranslationReq,
) bool {
	spec := m.comp.GetSpec()
	totalLatency := spec.LatencyPerLevel * uint64(spec.NumLevels)

	for level := spec.NumLevels - 1; level >= 0; level-- {
		found := m.lookupLevel(level, msg)
		if !found {
			break
		}
		totalLatency -= spec.LatencyPerLevel
	}

	ok := m.sendReqToBottom(msg, totalLatency)
	if !ok {
		return false
	}
	return true
}

func (m *mmuCacheMiddleware) lookupLevel(
	level int, req *vm.TranslationReq,
) bool {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	vAddr := req.VAddr
	pid := req.PID

	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	seg := (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)

	wayID, found := setLookup(&next.Table[level], pid, seg)

	if found {
		setVisit(&next.Table[level], wayID)
		return true
	}
	return false
}

func (m *mmuCacheMiddleware) sendReqToBottom(
	req *vm.TranslationReq,
	latency uint64) bool {
	if !m.bottomPort().CanSend() {
		return false
	}

	spec := m.comp.GetSpec()

	reqToBottom := &vm.TranslationReq{}
	reqToBottom.ID = sim.GetIDGenerator().Generate()
	reqToBottom.Src = m.bottomPort().AsRemote()
	reqToBottom.Dst = spec.LowModulePort
	reqToBottom.PID = req.PID
	reqToBottom.VAddr = req.VAddr
	reqToBottom.DeviceID = req.DeviceID
	reqToBottom.TransLatency = latency
	reqToBottom.TrafficClass = "vm.TranslationReq"

	err := m.bottomPort().Send(reqToBottom)
	if err != nil {
		return false
	}

	m.topPort().RetrieveIncoming()

	return true
}

func (m *mmuCacheMiddleware) handleBottomPort() bool {
	madeProgress := false

	itemI := m.bottomPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	switch item := itemI.(type) {
	case *vm.TranslationRsp:
		madeProgress = m.handleRsp(item) || madeProgress
	default:
		log.Panicf("cannot process request %s", fmt.Sprintf("%T", itemI))
	}
	return madeProgress
}

func (m *mmuCacheMiddleware) handleRsp(rsp *vm.TranslationRsp) bool {
	if !m.topPort().CanSend() {
		return false
	}

	m.updateCacheLevels(rsp)

	spec := m.comp.GetSpec()

	rspToTop := &vm.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = spec.UpModulePort
	rspToTop.RspTo = rsp.RspTo
	rspToTop.TrafficClass = "vm.TranslationRsp"

	err := m.topPort().Send(rspToTop)
	if err != nil {
		return false
	}

	m.bottomPort().RetrieveIncoming()

	return true
}

// segToSetID maps a segment to a cache set ID using modulo hashing.
func (m *mmuCacheMiddleware) segToSetID(seg uint64) int {
	spec := m.comp.GetSpec()
	return int(seg % uint64(spec.NumBlocks))
}

// updateCacheLevels updates all cache levels with the translation response.
func (m *mmuCacheMiddleware) updateCacheLevels(rsp *vm.TranslationRsp) bool {
	spec := m.comp.GetSpec()
	next := m.comp.GetNextState()
	page := rsp.Page
	vAddr := page.VAddr
	pid := page.PID

	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	for level := spec.NumLevels - 1; level >= 0; level-- {
		seg := (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)

		wayID := m.segToSetID(seg)

		_, found := setLookup(&next.Table[level], pid, seg)
		if found {
			continue
		}

		setUpdate(&next.Table[level], wayID, pid, seg)
	}

	return true
}

func (m *mmuCacheMiddleware) processMMUCacheFlush() bool {
	next := m.comp.GetNextState()
	spec := m.comp.GetSpec()

	rsp := &tlb.FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = next.InflightFlushReqSrc
	rsp.TrafficClass = "mmuCache.FlushRsp"

	err := m.controlPort().Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		next.InflightFlushReqID+"@"+m.comp.Name(),
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	// Reset table
	next.Table = initSets(spec.NumLevels, spec.NumBlocks)

	next.InflightFlushReqActive = false
	next.InflightFlushReqID = ""
	next.InflightFlushReqSrc = ""
	next.CurrentState = mmuCacheStatePause

	return true
}
