package mmuCache

import (
	"log"
	"reflect"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/tracing"
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
	for i := 0; i < cache.numReqPerCycle; i++ {
		madeProgress = cache.lookup() || madeProgress
	}
	for i := 0; i < cache.numReqPerCycle; i++ {
		madeProgress = cache.handleBottomPort() || madeProgress
	}
	return madeProgress
}

func (cache *mmuCacheMiddleware) lookup() bool {
	if !cache.bottomPort.CanSend() {
		return false
	}

	msg := cache.topPort.PeekIncoming()
	if msg == nil {
		return false
	}

	req, ok := msg.(*vm.TranslationReq)
	if !ok || req == nil {
		return false
	}

	return cache.walkCacheLevels(req)
}

func (cache *mmuCacheMiddleware) walkCacheLevels(req *vm.TranslationReq) bool {
	totalLatency := cache.latencyPerLevel * uint64(cache.numLevels)

	for level := cache.numLevels - 1; level >= 0; level-- {
		found := cache.lookupLevel(level, req)
		if !found {
			break
		}
		totalLatency -= cache.latencyPerLevel
	}

	ok := cache.sendReqToBottom(req, totalLatency)
	if !ok {
		return false
	}
	return true
}

func (cache *mmuCacheMiddleware) lookupLevel(level int, req *vm.TranslationReq) bool {
	vAddr := req.VAddr
	pid := req.PID

	vpn := vAddr >> cache.log2PageSize
	levelWidth := (64 - cache.log2PageSize) / uint64(cache.numLevels)
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

	reqToBottom := vm.TranslationReqBuilder{}.
		WithSrc(cache.bottomPort.AsRemote()).
		WithDst(cache.LowModule.AsRemote()).
		WithPID(req.PID).
		WithVAddr(req.VAddr).
		WithDeviceID(req.DeviceID).
		WithTransLatency(latency).
		Build()

	err := cache.bottomPort.Send(reqToBottom)
	if err != nil {
		return false
	}

	cache.topPort.RetrieveIncoming()

	return true
}

func (cache *mmuCacheMiddleware) handleBottomPort() bool {
	madeProgress := false

	item := cache.bottomPort.PeekIncoming()
	if item == nil {
		return false
	}

	switch rsp := item.(type) {
	case *vm.TranslationRsp:
		madeProgress = cache.handleRsp(rsp) || madeProgress
	default:
		log.Panicf("cannot process request %s", reflect.TypeOf(item))
	}
	return madeProgress
}

func (cache *mmuCacheMiddleware) handleRsp(rsp *vm.TranslationRsp) bool {
	if !cache.topPort.CanSend() {
		return false
	}

	cache.updateCacheLevels(rsp)

	rspToTop := vm.TranslationRspBuilder{}.
		WithSrc(cache.topPort.AsRemote()).
		WithDst(cache.UpModule.AsRemote()).
		WithRspTo(rsp.RespondTo).
		WithPage(rsp.Page).
		Build()

	err := cache.topPort.Send(rspToTop)
	if err != nil {
		return false
	}

	cache.bottomPort.RetrieveIncoming()

	return true
}

// segToSetID maps a segment to a cache set ID using modulo hashing.
func (cache *mmuCacheMiddleware) segToSetID(seg uint64) int {
	return int(seg % uint64(cache.numBlocks))
}

// updateCacheLevels updates all cache levels with the translation response.
func (cache *mmuCacheMiddleware) updateCacheLevels(rsp *vm.TranslationRsp) bool {
	page := rsp.Page
	vAddr := page.VAddr
	pid := page.PID

	vpn := vAddr >> cache.log2PageSize
	levelWidth := (64 - cache.log2PageSize) / uint64(cache.numLevels)
	for level := cache.numLevels - 1; level >= 0; level-- {
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

	rsp := FlushRspBuilder{}.
		WithSrc(cache.controlPort.AsRemote()).
		WithDst(req.Src).
		Build()

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
