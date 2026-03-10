package messaging

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v5/queueing"
	"github.com/sarchlab/akita/v5/sim"
)

// FlitPayload is the payload for a flit, the smallest transferring unit on a
// network.
type FlitPayload struct {
	SeqID        int
	NumFlitInMsg int
	Msg          *sim.Msg
	OutputBuf    queueing.Buffer // The buffer to route to within a switch
}

// FlitBuilder can build flits
type FlitBuilder struct {
	src, dst            sim.RemotePort
	msg                 *sim.Msg
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
func (b FlitBuilder) WithMsg(msg *sim.Msg) FlitBuilder {
	b.msg = msg
	return b
}

// Build creates a new *sim.Msg with FlitPayload.
func (b FlitBuilder) Build() *sim.Msg {
	payload := &FlitPayload{
		SeqID:        b.seqID,
		NumFlitInMsg: b.numFlitInMsg,
		Msg:          b.msg,
	}
	flitID := fmt.Sprintf("flit-%d-msg-%s-%s",
		b.seqID, b.msg.ID,
		sim.GetIDGenerator().Generate())

	var trafficClass string
	if b.msg.Payload != nil {
		msgValue := reflect.TypeOf(b.msg.Payload).Elem()
		trafficClass = msgValue.String()
	}

	return &sim.Msg{
		MsgMeta: sim.MsgMeta{
			ID:           flitID,
			Src:          b.src,
			Dst:          b.dst,
			TrafficClass: trafficClass,
		},
		Payload: payload,
	}
}
