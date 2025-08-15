package messaging

import (
	"fmt"
	"reflect"

	"github.com/sarchlab/akita/v4/sim"
)

// Flit is the smallest trasferring unit on a network.
type Flit struct {
	sim.MsgMeta

	SeqID        int
	NumFlitInMsg int
	Msg          sim.Msg
	OutputBuf    sim.Buffer // The buffer to route to within a switch
}

// Meta returns the meta data associated with the Flit.
func (f *Flit) Meta() *sim.MsgMeta {
	return &f.MsgMeta
}

// Clone returns cloned Flit with different ID
func (f *Flit) Clone() sim.Msg {
	cloneMsg := *f
	cloneMsg.ID = fmt.Sprintf("flit-%d-msg-%s-%s",
		cloneMsg.SeqID, cloneMsg.Msg.Meta().ID,
		sim.GetIDGenerator().Generate())

	return &cloneMsg
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

// Build creates a new flit.
func (b FlitBuilder) Build() *Flit {
	f := &Flit{}
	f.ID = fmt.Sprintf("flit-%d-msg-%s-%s",
		b.seqID, b.msg.Meta().ID,
		sim.GetIDGenerator().Generate())
	f.Src = b.src
	f.Dst = b.dst
	f.Msg = b.msg
	f.SeqID = b.seqID
	f.NumFlitInMsg = b.numFlitInMsg
	msgValue := reflect.TypeOf(f.Msg).Elem()
	f.TrafficClass = msgValue.String()

	return f
}
