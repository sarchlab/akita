package cache

import (
	"github.com/sarchlab/akita/v5/sim"
)

// FlushReq is a flush request sent to a cache unit to request it to flush all
// the cache lines.
type FlushReq struct {
	sim.MsgMeta
	InvalidateAllCachelines bool
	DiscardInflight         bool
	PauseAfterFlushing      bool
}

// FlushReqBuilder can build flush requests.
type FlushReqBuilder struct {
	src, dst                sim.RemotePort
	invalidateAllCacheLines bool
	discardInflight         bool
	pauseAfterFlushing      bool
}

// WithSrc sets the source of the message to build
func (b FlushReqBuilder) WithSrc(src sim.RemotePort) FlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message to build.
func (b FlushReqBuilder) WithDst(dst sim.RemotePort) FlushReqBuilder {
	b.dst = dst
	return b
}

// InvalidateAllCacheLines allows the flush request to build to invalidate
// all the cachelines in a cache unit.
func (b FlushReqBuilder) InvalidateAllCacheLines() FlushReqBuilder {
	b.invalidateAllCacheLines = true
	return b
}

// DiscardInflight allows the flush request to build to discard all inflight
// requests.
func (b FlushReqBuilder) DiscardInflight() FlushReqBuilder {
	b.discardInflight = true
	return b
}

// PauseAfterFlushing sets the flush request to build to pause the cache unit
// from processing future request until restart request is received.
func (b FlushReqBuilder) PauseAfterFlushing() FlushReqBuilder {
	b.pauseAfterFlushing = true
	return b
}

// Build creates a new FlushReq.
func (b FlushReqBuilder) Build() *FlushReq {
	r := &FlushReq{
		InvalidateAllCachelines: b.invalidateAllCacheLines,
		DiscardInflight:         b.discardInflight,
		PauseAfterFlushing:      b.pauseAfterFlushing,
	}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "cache.FlushReq"
	return r
}

// FlushRsp is a response indicating a cache flush is complete.
type FlushRsp struct {
	sim.MsgMeta
}

// FlushRspBuilder can build flush responds.
type FlushRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b FlushRspBuilder) WithSrc(src sim.RemotePort) FlushRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b FlushRspBuilder) WithDst(dst sim.RemotePort) FlushRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b FlushRspBuilder) WithRspTo(id string) FlushRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new FlushRsp.
func (b FlushRspBuilder) Build() *FlushRsp {
	r := &FlushRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo
	r.TrafficClass = "cache.FlushRsp"
	return r
}

// RestartReq is a restart request sent to a cache unit to unpause it.
type RestartReq struct {
	sim.MsgMeta
}

// RestartReqBuilder can build restart requests.
type RestartReqBuilder struct {
	src, dst sim.RemotePort
}

// WithSrc sets the source of the request to build.
func (b RestartReqBuilder) WithSrc(src sim.RemotePort) RestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartReqBuilder) WithDst(dst sim.RemotePort) RestartReqBuilder {
	b.dst = dst
	return b
}

// Build creates a new RestartReq.
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "cache.RestartReq"
	return r
}

// RestartRsp is a response indicating a cache restart is complete.
type RestartRsp struct {
	sim.MsgMeta
}

// RestartRspBuilder can build restart responds.
type RestartRspBuilder struct {
	src, dst sim.RemotePort
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b RestartRspBuilder) WithSrc(src sim.RemotePort) RestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartRspBuilder) WithDst(dst sim.RemotePort) RestartRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b RestartRspBuilder) WithRspTo(id string) RestartRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new RestartRsp.
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo
	r.TrafficClass = "cache.RestartRsp"
	return r
}
