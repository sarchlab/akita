package messaging

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// Flit is a concrete message representing the smallest transferring unit on a
// network.
type Flit struct {
	sim.MsgMeta
	SeqID        int
	NumFlitInMsg int
	Msg          sim.Msg
	OutputBuf    queueing.Buffer // The buffer to route to within a switch
}

// FlitBuilder can build flits
type FlitBuilder struct {
	src, dst            sim.RemotePort
	msg                 sim.Msg
	seqID, numFlitInMsg int
}

// WithSrc sets the src of the request to send
func (b FlitBuilder) WithSrc(src sim.RemotePort) FlitBuilder {
	b.src = src
	return b
}

// WithDst sets the dst of the request to send
func (b FlitBuilder) WithDst(dst sim.RemotePort) FlitBuilder {
	b.dst = dst
	return b
}

// WithSeqID sets the SeqID of the Flit.
func (b FlitBuilder) WithSeqID(i int) FlitBuilder {
	b.seqID = i
	return b
}

// WithNumFlitInMsg sets the NumFlitInMsg for of flit to build.
func (b FlitBuilder) WithNumFlitInMsg(n int) FlitBuilder {
	b.numFlitInMsg = n
	return b
}

// WithMsg sets the msg of the flit to build.
func (b FlitBuilder) WithMsg(msg sim.Msg) FlitBuilder {
	b.msg = msg
	return b
}

// Build creates a new *Flit.
func (b FlitBuilder) Build() *Flit {
	flitID := fmt.Sprintf("flit-%d-msg-%s-%s",
		b.seqID, b.msg.Meta().ID,
		sim.GetIDGenerator().Generate())

	trafficClass := reflect.TypeOf(b.msg).String()

	return &Flit{
		MsgMeta: sim.MsgMeta{
			ID:           flitID,
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: trafficClass,
		},
		SeqID:        b.seqID,
		NumFlitInMsg: b.numFlitInMsg,
		Msg:          b.msg,
	}
}
