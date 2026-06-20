package mmuCache

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
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

func (m *mmuCacheMiddleware) Tick() bool {
	madeProgress := false
	next := &m.comp.State

	switch next.CurrentState {
	case mmuCacheStateDrain:
		madeProgress = m.handleDrain() || madeProgress
	case mmuCacheStatePause:
		return false
	default:
		madeProgress = m.handleEnable() || madeProgress
	}
	return madeProgress
}

func (m *mmuCacheMiddleware) handleDrain() bool {
	// Draining retires already-forwarded walks but admits no new Top traffic,
	// per the protocol's "Drain stops accepting new traffic". Quiescence is
	// based only on in-flight work (bottom responses + outstanding walks), not
	// on the Top queue, so the drain converges even if upstream keeps queuing;
	// those queued requests resume after Enable.
	madeProgress := false
	spec := m.comp.Spec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = m.handleBottomPort() || madeProgress
	}

	next := &m.comp.State
	quiescent := m.bottomPort().PeekIncoming() == nil &&
		len(next.OutstandingBottomReqs) == 0
	if quiescent {
		next.CurrentState = mmuCacheStatePause
	}

	return madeProgress
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

	msg, ok := msgI.(vmprotocol.TranslationReq)
	if !ok {
		return false
	}

	return m.walkCacheLevels(msg)
}

func (m *mmuCacheMiddleware) walkCacheLevels(
	msg vmprotocol.TranslationReq,
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
	level int, req vmprotocol.TranslationReq,
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
	req vmprotocol.TranslationReq,
	latency uint64) bool {
	if !m.bottomPort().CanSend() {
		return false
	}

	res := m.comp.Resources()

	reqToBottom := vmprotocol.TranslationReq{}
	reqToBottom.ID = timing.GetIDGenerator().Generate()
	reqToBottom.Src = m.bottomPort().AsRemote()
	reqToBottom.Dst = res.LowModulePort
	reqToBottom.PID = req.PID
	reqToBottom.VAddr = req.VAddr
	reqToBottom.DeviceID = req.DeviceID
	reqToBottom.TransLatency = latency
	reqToBottom.TrafficClass = "vmprotocol.TranslationReq"

	m.bottomPort().Send(reqToBottom)
	m.comp.State.OutstandingBottomReqs[reqToBottom.ID] = true

	// The forwarded walk is on its way downstream: the at-head wait to send on
	// the Bottom port is over. This admission wait belongs to the Top
	// incoming-buffer task; req_in (opened just below, at retrieve) covers only
	// the processing that follows.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(req, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.bottomPort().Name(),
	})

	m.topPort().RetrieveIncoming()

	// Open req_in for the original Top request at retrieve, and initiate the
	// forwarded Bottom request as a req_out parented to it. The Top request's
	// identity is retained (keyed by the Bottom request ID) so the req_in and
	// req_out can be completed when the response returns — the completion path
	// only has message IDs, not the live messages.
	tracing.TraceReqReceive(m.comp, req)
	tracing.TraceReqInitiate(m.comp, reqToBottom,
		tracing.MsgIDAtReceiver(req, m.comp))

	m.comp.State.InflightReqs[reqToBottom.ID] = inflightReqState{
		TopReqID:     req.ID,
		TopReqSrc:    req.Src,
		TopReqDst:    req.Dst,
		BottomReqID:  reqToBottom.ID,
		BottomReqSrc: reqToBottom.Src,
		BottomReqDst: reqToBottom.Dst,
	}

	return true
}

func (m *mmuCacheMiddleware) handleBottomPort() bool {
	madeProgress := false

	itemI := m.bottomPort().PeekIncoming()
	if itemI == nil {
		return false
	}

	switch item := itemI.(type) {
	case vmprotocol.TranslationRsp:
		madeProgress = m.handleRsp(item) || madeProgress
	default:
		log.Panicf("cannot process request %s", fmt.Sprintf("%T", itemI))
	}
	return madeProgress
}

func (m *mmuCacheMiddleware) handleRsp(rsp vmprotocol.TranslationRsp) bool {
	next := &m.comp.State
	if _, live := next.OutstandingBottomReqs[rsp.RspTo]; !live {
		// Orphaned response: its forwarded request was dropped (e.g. a Reset
		// landed mid-walk). Discard it instead of repopulating the freshly
		// reset table or emitting a stale translation upward.
		m.bottomPort().RetrieveIncoming()
		return true
	}

	if !m.topPort().CanSend() {
		return false
	}

	m.updateCacheLevels(rsp)

	res := m.comp.Resources()

	rspToTop := vmprotocol.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = timing.GetIDGenerator().Generate()
	rspToTop.Src = m.topPort().AsRemote()
	rspToTop.Dst = res.UpModulePort
	rspToTop.RspTo = rsp.RspTo
	rspToTop.TrafficClass = "vmprotocol.TranslationRsp"

	m.topPort().Send(rspToTop)

	// The response had reached the head of the Bottom buffer and was held there
	// until the Top port could accept the upstream response. That at-head
	// send-wait is over now; attribute it to the Bottom incoming-buffer task.
	// This must precede RetrieveIncoming: retrieving closes the buffer task and
	// forgets its registry ID, after which MsgIDAtIncomingBuffer would mint a
	// fresh, unrelated ID.
	tracing.AddMilestone(m.comp, tracing.Milestone{
		TaskID: tracing.MsgIDAtIncomingBuffer(rsp, m.comp),
		Kind:   tracing.MilestoneKindNetworkBusy,
		What:   m.topPort().Name(),
	})

	m.bottomPort().RetrieveIncoming()
	delete(next.OutstandingBottomReqs, rsp.RspTo)

	// The upstream response is on its way: finalize the forwarded req_out and
	// complete the original req_in. The completion path only retains message
	// IDs, so both messages are reconstructed from the saved identity.
	if inflight, ok := next.InflightReqs[rsp.RspTo]; ok {
		reqToBottom := restoreTransReq(
			inflight.BottomReqID, inflight.BottomReqSrc, inflight.BottomReqDst)
		topReq := restoreTransReq(
			inflight.TopReqID, inflight.TopReqSrc, inflight.TopReqDst)
		tracing.TraceReqFinalize(m.comp, reqToBottom)
		tracing.TraceReqComplete(m.comp, topReq)
		delete(next.InflightReqs, rsp.RspTo)
	}

	return true
}

// segToSetID maps a segment to a cache set ID using modulo hashing.
func (m *mmuCacheMiddleware) segToSetID(seg uint64) int {
	spec := m.comp.Spec()
	return int(seg % uint64(spec.NumBlocks))
}

// updateCacheLevels updates all cache levels with the translation response.
func (m *mmuCacheMiddleware) updateCacheLevels(rsp vmprotocol.TranslationRsp) bool {
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
