package cache

import (
	"github.com/rs/xid"
	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
)

// FlushReq is the request send to a cache unit to request it to flush all
// the cache lines.
type FlushReq struct {
	modeling.MsgMeta
	InvalidateAllCachelines bool
	DiscardInflight         bool
	PauseAfterFlushing      bool
}

// Meta returns the meta data associated with the message.
func (r *FlushReq) Meta() *modeling.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned FlushReq with different ID
func (r *FlushReq) Clone() modeling.Msg {
	cloneMsg := *r
	cloneMsg.ID = id.Generate()

	return &cloneMsg
}

func (r *FlushReq) GenerateRsp() modeling.Rsp {
	rsp := FlushRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		Build()

	return rsp
}

// FlushReqBuilder can build flush requests.
type FlushReqBuilder struct {
	src, dst                modeling.RemotePort
	invalidateAllCacheLines bool
	discardInflight         bool
	pauseAfterFlushing      bool
}

// WithSrc sets the source of the message to build
func (b FlushReqBuilder) WithSrc(src modeling.RemotePort) FlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the message to build.
func (b FlushReqBuilder) WithDst(dst modeling.RemotePort) FlushReqBuilder {
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

// Build creates a new FlushReq
func (b FlushReqBuilder) Build() *FlushReq {
	r := &FlushReq{}
	r.ID = id.Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.InvalidateAllCachelines = b.invalidateAllCacheLines
	r.DiscardInflight = b.discardInflight
	r.PauseAfterFlushing = b.pauseAfterFlushing

	return r
}

// FlushRsp is the respond sent from the a cache unit for finishing a cache
// flush
type FlushRsp struct {
	modeling.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r *FlushRsp) Meta() *modeling.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned FlushRsp with different ID
func (r *FlushRsp) Clone() modeling.Msg {
	cloneMsg := *r
	cloneMsg.ID = id.Generate()

	return &cloneMsg
}

func (r *FlushRsp) GetRspTo() string {
	return r.RspTo
}

// FlushRspBuilder can build data ready responds.
type FlushRspBuilder struct {
	src, dst modeling.RemotePort
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b FlushRspBuilder) WithSrc(src modeling.RemotePort) FlushRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b FlushRspBuilder) WithDst(dst modeling.RemotePort) FlushRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b FlushRspBuilder) WithRspTo(id string) FlushRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new FlushRsp
func (b FlushRspBuilder) Build() *FlushRsp {
	r := &FlushRsp{}
	r.ID = id.Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo

	return r
}

// RestartReq is the request send to a cache unit to request it unpause the
// cache
type RestartReq struct {
	modeling.MsgMeta
}

// Meta returns the meta data associated with the message.
func (r *RestartReq) Meta() *modeling.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned RestartReq with different ID
func (r *RestartReq) Clone() modeling.Msg {
	cloneMsg := *r
	cloneMsg.ID = id.Generate()

	return &cloneMsg
}

func (r *RestartReq) GenerateRsp() modeling.Rsp {
	rsp := RestartRspBuilder{}.
		WithSrc(r.Dst).
		WithDst(r.Src).
		WithRspTo(r.ID).
		Build()

	return rsp
}

// RestartReqBuilder can build data ready responds.
type RestartReqBuilder struct {
	src, dst modeling.RemotePort
}

// WithSrc sets the source of the request to build.
func (b RestartReqBuilder) WithSrc(src modeling.RemotePort) RestartReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartReqBuilder) WithDst(dst modeling.RemotePort) RestartReqBuilder {
	b.dst = dst
	return b
}

// Build creates a new RestartReq
func (b RestartReqBuilder) Build() *RestartReq {
	r := &RestartReq{}
	r.ID = id.Generate()
	r.Src = b.src
	r.Dst = b.dst

	return r
}

// RestartRsp is the respond sent from the a cache unit for finishing a cache
// flush
type RestartRsp struct {
	modeling.MsgMeta
	RspTo string
}

// Meta returns the meta data associated with the message.
func (r *RestartRsp) Meta() *modeling.MsgMeta {
	return &r.MsgMeta
}

// Clone returns cloned RestartRsp with different ID
func (r *RestartRsp) Clone() modeling.Msg {
	cloneMsg := *r
	cloneMsg.ID = xid.New().String()

	return &cloneMsg
}

func (r *RestartRsp) GetRspTo() string {
	return r.RspTo
}

// RestartRspBuilder can build data ready responds.
type RestartRspBuilder struct {
	src, dst modeling.RemotePort
	rspTo    string
}

// WithSrc sets the source of the request to build.
func (b RestartRspBuilder) WithSrc(src modeling.RemotePort) RestartRspBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b RestartRspBuilder) WithDst(dst modeling.RemotePort) RestartRspBuilder {
	b.dst = dst
	return b
}

// WithRspTo sets ID of the request that the respond to build is replying to.
func (b RestartRspBuilder) WithRspTo(id string) RestartRspBuilder {
	b.rspTo = id
	return b
}

// Build creates a new RestartRsp
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = xid.New().String()
	r.Src = b.src
	r.Dst = b.dst
	r.RspTo = b.rspTo

	return r
}
