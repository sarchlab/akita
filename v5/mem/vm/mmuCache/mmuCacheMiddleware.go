package mmuCache

import (
	"fmt"
	"log"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/tracing"
)

const (
	mmuCacheStateEnable = "enable"
	mmuCacheStatePause  = "pause"
	mmuCacheStateDrain  = "drain"
	mmuCacheStateFlush  = "flush"
)

type mmuCacheMiddleware struct {
	*Comp
}

func (cache *mmuCacheMiddleware) Tick() bool {
	madeProgress := false

	switch cache.state {
	case mmuCacheStateDrain:
		madeProgress = cache.handleDrain() || madeProgress
	case mmuCacheStatePause:
		return false
	case mmuCacheStateFlush:
		madeProgress = cache.handleFlush() || madeProgress
	default:
		madeProgress = cache.handleEnable() || madeProgress
	}
	return madeProgress
}

func (cache *mmuCacheMiddleware) handleDrain() bool {
	madeProgress := cache.processRequests()

	if cache.bottomPort.PeekIncoming() == nil && cache.topPort.PeekIncoming() == nil {
		cache.state = mmuCacheStatePause
		tracing.AddMilestone(
			cache.Comp.Name()+".drain",
			tracing.MilestoneKindHardwareResource,
			cache.Comp.Name()+".",
			cache.Comp.Name(),
			cache.Comp,
		)
	}

	return madeProgress
}

func (cache *mmuCacheMiddleware) handleFlush() bool {
	if cache.inflightFlushReq == nil {
		return false
	}

	if cache.topPort.PeekIncoming() == nil && cache.bottomPort.PeekIncoming() == nil {
		return cache.processMMUCacheFlush()
	}

	return cache.processRequests()
}

// handleEnable processes requests when cache is in enabled state.
func (cache *mmuCacheMiddleware) handleEnable() bool {
	return cache.processRequests()
}

// processRequests handles both incoming lookup requests and bottom port responses.
func (cache *mmuCacheMiddleware) processRequests() bool {
	madeProgress := false
	spec := cache.GetSpec()
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = cache.lookup() || madeProgress
	}
	for i := 0; i < spec.NumReqPerCycle; i++ {
		madeProgress = cache.handleBottomPort() || madeProgress
	}
	return madeProgress
}

func (cache *mmuCacheMiddleware) lookup() bool {
	if !cache.bottomPort.CanSend() {
		return false
	}

	msgI := cache.topPort.PeekIncoming()
	if msgI == nil {
		return false
	}

	msg, ok := msgI.(*vm.TranslationReq)
	if !ok || msg == nil {
		return false
	}

	return cache.walkCacheLevels(msg)
}

func (cache *mmuCacheMiddleware) walkCacheLevels(
	msg *vm.TranslationReq,
) bool {
	spec := cache.GetSpec()
	totalLatency := spec.LatencyPerLevel * uint64(spec.NumLevels)

	for level := spec.NumLevels - 1; level >= 0; level-- {
		found := cache.lookupLevel(level, msg)
		if !found {
			break
		}
		totalLatency -= spec.LatencyPerLevel
	}

	ok := cache.sendReqToBottom(msg, totalLatency)
	if !ok {
		return false
	}
	return true
}

func (cache *mmuCacheMiddleware) lookupLevel(
	level int, req *vm.TranslationReq,
) bool {
	spec := cache.GetSpec()
	vAddr := req.VAddr
	pid := req.PID

	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	seg := (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)

	subTable := cache.table[level]
	wayID, found := subTable.Lookup(pid, seg)

	if found {
		subTable.Visit(wayID)
		return true
	}
	return false
}

func (cache *mmuCacheMiddleware) sendReqToBottom(
	req *vm.TranslationReq,
	latency uint64) bool {
	if !cache.bottomPort.CanSend() {
		return false
	}

	reqToBottom := &vm.TranslationReq{}
	reqToBottom.ID = sim.GetIDGenerator().Generate()
	reqToBottom.Src = cache.bottomPort.AsRemote()
	reqToBottom.Dst = cache.LowModule.AsRemote()
	reqToBottom.PID = req.PID
	reqToBottom.VAddr = req.VAddr
	reqToBottom.DeviceID = req.DeviceID
	reqToBottom.TransLatency = latency
	reqToBottom.TrafficClass = "vm.TranslationReq"

	err := cache.bottomPort.Send(reqToBottom)
	if err != nil {
		return false
	}

	cache.topPort.RetrieveIncoming()

	return true
}

func (cache *mmuCacheMiddleware) handleBottomPort() bool {
	madeProgress := false

	itemI := cache.bottomPort.PeekIncoming()
	if itemI == nil {
		return false
	}

	switch item := itemI.(type) {
	case *vm.TranslationRsp:
		madeProgress = cache.handleRsp(item) || madeProgress
	default:
		log.Panicf("cannot process request %s", fmt.Sprintf("%T", itemI))
	}
	return madeProgress
}

func (cache *mmuCacheMiddleware) handleRsp(rsp *vm.TranslationRsp) bool {
	if !cache.topPort.CanSend() {
		return false
	}

	cache.updateCacheLevels(rsp)

	rspToTop := &vm.TranslationRsp{
		Page: rsp.Page,
	}
	rspToTop.ID = sim.GetIDGenerator().Generate()
	rspToTop.Src = cache.topPort.AsRemote()
	rspToTop.Dst = cache.UpModule.AsRemote()
	rspToTop.RspTo = rsp.RspTo
	rspToTop.TrafficClass = "vm.TranslationRsp"

	err := cache.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	cache.bottomPort.RetrieveIncoming()

	return true
}

// segToSetID maps a segment to a cache set ID using modulo hashing.
func (cache *mmuCacheMiddleware) segToSetID(seg uint64) int {
	spec := cache.GetSpec()
	return int(seg % uint64(spec.NumBlocks))
}

// updateCacheLevels updates all cache levels with the translation response.
func (cache *mmuCacheMiddleware) updateCacheLevels(rsp *vm.TranslationRsp) bool {
	spec := cache.GetSpec()
	page := rsp.Page
	vAddr := page.VAddr
	pid := page.PID

	vpn := vAddr >> spec.Log2PageSize
	levelWidth := (64 - spec.Log2PageSize) / uint64(spec.NumLevels)
	for level := spec.NumLevels - 1; level >= 0; level-- {
		seg := (vpn >> (uint64(level) * levelWidth)) & ((1 << levelWidth) - 1)

		subTable := cache.table[level]
		wayID := cache.segToSetID(seg)

		_, found := subTable.Lookup(pid, seg)
		if found {
			continue
		}

		subTable.Update(wayID, pid, seg)
	}

	return true
}

func (cache *mmuCacheMiddleware) processMMUCacheFlush() bool {
	req := cache.inflightFlushReq

	rsp := &FlushRsp{}
	rsp.ID = sim.GetIDGenerator().Generate()
	rsp.Src = cache.controlPort.AsRemote()
	rsp.Dst = req.Src
	rsp.TrafficClass = "mmuCache.FlushRsp"

	err := cache.controlPort.Send(rsp)
	if err != nil {
		return false
	}
	tracing.AddMilestone(
		tracing.MsgIDAtReceiver(req, cache.Comp),
		tracing.MilestoneKindNetworkBusy,
		cache.controlPort.Name(),
		cache.Comp.Name(),
		cache.Comp,
	)

	cache.reset()

	cache.inflightFlushReq = nil
	cache.state = mmuCacheStatePause

	return true
}
