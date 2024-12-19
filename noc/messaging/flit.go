package messaging

import (
	"fmt"

	"github.com/sarchlab/akita/v4/sim/id"
	"github.com/sarchlab/akita/v4/sim/modeling"
	"github.com/sarchlab/akita/v4/sim/queueing"
)

// Flit is the smallest trasferring unit on a network.
type Flit struct {
	modeling.MsgMeta
	SeqID        int
	NumFlitInMsg int
	Msg          modeling.Msg
	OutputBuf    queueing.Buffer // The buffer to route to within a switch
}

// ID returns the ID of the Flit.
func (f Flit) ID() string {
	return f.MsgMeta.ID
}

// Serialize serializes the Flit.
func (f Flit) Serialize() (map[string]any, error) {
	return map[string]any{
		"src":             f.Src,
		"dst":             f.Dst,
		"traffic_class":   f.TrafficClass,
		"traffic_bytes":   f.TrafficBytes,
		"seq_id":          f.SeqID,
		"num_flit_in_msg": f.NumFlitInMsg,
		"msg":             f.Msg,
	}, nil
}

// Deserialize deserializes the Flit.
func (f *Flit) Deserialize(
	data map[string]any,
) error {
	f.Src = data["src"].(modeling.RemotePort)
	f.Dst = data["dst"].(modeling.RemotePort)
	f.TrafficClass = data["traffic_class"].(int)
	f.TrafficBytes = data["traffic_bytes"].(int)
	f.SeqID = data["seq_id"].(int)
	f.NumFlitInMsg = data["num_flit_in_msg"].(int)
	f.Msg = data["msg"].(modeling.Msg)

	return nil
}

// Meta returns the meta data associated with the Flit.
func (f Flit) Meta() modeling.MsgMeta {
	return f.MsgMeta
}

// Clone returns cloned Flit with different ID
func (f Flit) Clone() modeling.Msg {
	cloneMsg := f
	cloneMsg.MsgMeta.ID = fmt.Sprintf("flit-%d-msg-%s-%s",
		cloneMsg.SeqID, cloneMsg.Msg.Meta().ID,
		id.Generate())

	return &cloneMsg
}

// FlitBuilder can build flits
type FlitBuilder struct {
	src, dst            modeling.RemotePort
	msg                 modeling.Msg
	seqID, numFlitInMsg int
}

// WithSrc sets the src of the request to send
func (b FlitBuilder) WithSrc(src modeling.RemotePort) FlitBuilder {
	b.src = src
	return b
}

// WithDst sets the dst of the request to send
func (b FlitBuilder) WithDst(dst modeling.RemotePort) FlitBuilder {
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
func (b FlitBuilder) WithMsg(msg modeling.Msg) FlitBuilder {
	b.msg = msg
	return b
}

// Build creates a new flit.
func (b FlitBuilder) Build() *Flit {
	f := &Flit{}
	f.MsgMeta.ID = fmt.Sprintf("flit-%d-msg-%s-%s",
		b.seqID, b.msg.Meta().ID,
		id.Generate())
	f.Src = b.src
	f.Dst = b.dst
	f.Msg = b.msg
	f.SeqID = b.seqID
	f.NumFlitInMsg = b.numFlitInMsg

	return f
}
