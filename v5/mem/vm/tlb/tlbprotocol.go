package tlb

import (
	"reflect"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim"
)

// FlushReqPayload is the payload for a TLB flush request to invalidate certain
// entries.
type FlushReqPayload struct {
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

// Build creates a new *sim.GenericMsg with FlushReqPayload.
func (b FlushReqBuilder) Build() *sim.GenericMsg {
	payload := &FlushReqPayload{
		VAddr: b.vAddrs,
		PID:   b.pid,
	}
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: reflect.TypeOf(FlushReqPayload{}).String(),
		},
		Payload: payload,
	}
}

// FlushRspPayload is the payload for a TLB flush response.
type FlushRspPayload struct{}

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

// Build creates a new *sim.GenericMsg with FlushRspPayload.
func (b FlushRspBuilder) Build() *sim.GenericMsg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: reflect.TypeOf(FlushReqPayload{}).String(),
		},
		Payload: &FlushRspPayload{},
	}
}

// RestartReqPayload is the payload for a TLB restart request.
type RestartReqPayload struct{}

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

// Build creates a new *sim.GenericMsg with RestartReqPayload.
func (b RestartReqBuilder) Build() *sim.GenericMsg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: reflect.TypeOf(RestartReqPayload{}).String(),
		},
		Payload: &RestartReqPayload{},
	}
}

// RestartRspPayload is the payload for a TLB restart response.
type RestartRspPayload struct{}

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

// Build creates a new *sim.GenericMsg with RestartRspPayload.
func (b RestartRspBuilder) Build() *sim.GenericMsg {
	return &sim.GenericMsg{
		MsgMeta: sim.MsgMeta{
			ID:           sim.GetIDGenerator().Generate(),
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: reflect.TypeOf(RestartReqPayload{}).String(),
		},
		Payload: &RestartRspPayload{},
	}
}
