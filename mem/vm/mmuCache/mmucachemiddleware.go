package mmuCache

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/modeling"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
)

type mmuCacheMiddleware struct {
	comp *modeling.Component[Spec, State, Resources]
}

func (m *mmuCacheMiddleware) topPort() messaging.Port {
	return m.comp.GetPortByName("Top")
}

func (m *mmuCacheMiddleware) bottomPort() messaging.Port {
	return m.comp.GetPortByName("Bottom")
}

func (m *mmuCacheMiddleware) controlPort() messaging.Port {
	return m.comp.GetPortByName("Control")
}

func (m *mmuCacheMiddleware) Tick() bool {
	madeProgress := false
	next := &m.comp.State

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
		next := &m.comp.State
		next.CurrentState = mmuCacheStatePause
		tracing.AddMilestone(
			timing.GetIDGenerator().Generate(),
			tracing.MilestoneKindHardwareResource,
			m.comp.Name()+".",
			m.comp.Name(),
			m.comp,
		)
	}

	return madeProgress
}

func (m *mmuCacheMiddleware) handleFlush() bool {
	next := &m.comp.State
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
	spec := m.comp.Spec()
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
	spec := m.comp.Spec()
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
	spec := m.comp.Spec()
	next := &m.comp.State
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

	res := m.comp.Resources()

	reqToBottom := &vm.TranslationReq{}
	reqToBottom.ID = timing.GetIDGenerator().Generate()
	reqToBottom.Src = m.bottomPort().AsRemote()
	reqToBottom.Dst = res.LowModulePort
	reqToBottom.PID = req.PID
	reqToBottom.VAddr = req.VAddr
	reqToBottom.DeviceID = req.DeviceID
	reqToBottom.TransLatency = latency
	reqToBottom.TrafficClass = "vm.TranslationReq"

	m.bottomPort().Send(reqToBottom)

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

	res := m.comp.Resources()

	rspToTop := &vm.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = timing.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = res.UpModulePort
	rspToTop.RspTo = rsp.RspTo
	rspToTop.TrafficClass = "vm.TranslationRsp"

	m.topPort().Send(rspToTop)

	m.bottomPort().RetrieveIncoming()

	return true
}

// segToSetID maps a segment to a cache set ID using modulo hashing.
func (m *mmuCacheMiddleware) segToSetID(seg uint64) int {
	spec := m.comp.Spec()
	return int(seg % uint64(spec.NumBlocks))
}

// updateCacheLevels updates all cache levels with the translation response.
func (m *mmuCacheMiddleware) updateCacheLevels(rsp *vm.TranslationRsp) bool {
	spec := m.comp.Spec()
	next := &m.comp.State
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
	next := &m.comp.State
	spec := m.comp.Spec()

	rsp := &mem.ControlRsp{Command: mem.CmdFlush, Success: true}
	rsp.ID = timing.GetIDGenerator().Generate()
	rsp.Src = m.controlPort().AsRemote()
	rsp.Dst = next.InflightFlushReqSrc
	rsp.RspTo = next.InflightFlushReqID
	rsp.TrafficClass = "mem.ControlRsp"

	if !m.controlPort().CanSend() {
		return false
	}

	m.controlPort().Send(rsp)
	tracing.AddMilestone(
		next.InflightFlushReqID,
		tracing.MilestoneKindNetworkBusy,
		m.controlPort().Name(),
		m.comp.Name(),
		m.comp,
	)

	// Reset table
	next.Table = initSets(spec.NumLevels, spec.NumBlocks)

	next.InflightFlushReqActive = false
	next.InflightFlushReqID = 0
	next.InflightFlushReqSrc = ""
	next.CurrentState = mmuCacheStatePause

	return true
}
