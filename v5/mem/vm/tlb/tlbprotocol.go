package tlb

import (
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// FlushReq is a TLB flush request to invalidate certain entries.
type FlushReq struct {
	sim.MsgMeta
	VAddr []uint64
	PID   vm.PID
}

// FlushReqBuilder can build TLB flush requests
type FlushReqBuilder struct {
	src, dst sim.RemotePort
	vAddrs   []uint64
	pid      vm.PID
}

// WithSrc sets the source of the request to build.
func (b FlushReqBuilder) WithSrc(src sim.RemotePort) FlushReqBuilder {
	b.src = src
	return b
}

// WithDst sets the destination of the request to build.
func (b FlushReqBuilder) WithDst(dst sim.RemotePort) FlushReqBuilder {
	b.dst = dst
	return b
}

// WithVAddrs sets the Vaddr of the pages to be flushed
func (b FlushReqBuilder) WithVAddrs(vAddrs []uint64) FlushReqBuilder {
	b.vAddrs = vAddrs
	return b
}

// WithPID sets the pid whose entries are to be flushed
func (b FlushReqBuilder) WithPID(pid vm.PID) FlushReqBuilder {
	b.pid = pid
	return b
}

// Build creates a new FlushReq.
func (b FlushReqBuilder) Build() *FlushReq {
	r := &FlushReq{
		VAddr: b.vAddrs,
		PID:   b.pid,
	}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "tlb.FlushReq"
	return r
}

// FlushRsp is a TLB flush response.
type FlushRsp struct {
	sim.MsgMeta
}

// FlushRspBuilder can build TLB flush responses
type FlushRspBuilder struct {
	src, dst sim.RemotePort
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

// Build creates a new FlushRsp.
func (b FlushRspBuilder) Build() *FlushRsp {
	r := &FlushRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "tlb.FlushRsp"
	return r
}

// RestartReq is a TLB restart request.
type RestartReq struct {
	sim.MsgMeta
}

// RestartReqBuilder can build TLB restart requests.
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
	r.TrafficClass = "tlb.RestartReq"
	return r
}

// RestartRsp is a TLB restart response.
type RestartRsp struct {
	sim.MsgMeta
}

// RestartRspBuilder can build TLB restart responses
type RestartRspBuilder struct {
	src, dst sim.RemotePort
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

// Build creates a new RestartRsp.
func (b RestartRspBuilder) Build() *RestartRsp {
	r := &RestartRsp{}
	r.ID = sim.GetIDGenerator().Generate()
	r.Src = b.src
	r.Dst = b.dst
	r.TrafficClass = "tlb.RestartRsp"
	return r
}
