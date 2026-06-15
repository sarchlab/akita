package packetization

import (
	"encoding/json"

	"github.com/sarchlab/akita/v5/messaging"
)

// Protocol is the flit transport protocol. On the link role, endpoints and
// switches exchange flits over network links (symmetric link traffic).
// Defining the protocol registers the flit type with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("packetization",
		messaging.RoleDef{Name: "link",
			Sends: []messaging.Msg{Flit{}}},
	)
	Link = Protocol.Role("link")
)

// Flit is a concrete message representing the smallest transferring unit on a
// network. An endpoint splits each outgoing message into one or more flits
// (count derived from the message's TrafficBytes) and the receiving endpoint
// reassembles them.
//
// Every flit carries the original message's routing metadata in Msg, so each
// switch can route a flit independently and the receiving endpoint can group
// flits by message ID. The original concrete message itself rides on the final
// flit, in Payload. The receiving endpoint delivers that concrete message — not
// a metadata-only stand-in — so payload-bearing protocols survive the network
// crossing while the flit count still models traffic and timing.
type Flit struct {
	messaging.MsgMeta
	SeqID        int               `json:"seq_id"`
	NumFlitInMsg int               `json:"num_flit_in_msg"`
	Msg          messaging.MsgMeta `json:"msg"`     // carried message metadata (every flit)
	Payload      messaging.Msg     `json:"payload"` // carried message (final flit only)
}

// flitJSON is the serialized form of a Flit. Hop holds the flit's own
// hop-routing metadata (the embedded MsgMeta); it is a named field rather than
// an embedded one so flitJSON does not itself satisfy messaging.Msg. Payload is
// encoded through the message codec so its concrete type survives a checkpoint.
type flitJSON struct {
	Hop          messaging.MsgMeta `json:"hop"`
	SeqID        int               `json:"seq_id"`
	NumFlitInMsg int               `json:"num_flit_in_msg"`
	Msg          messaging.MsgMeta `json:"msg"`
	Payload      json.RawMessage   `json:"payload"`
}

// MarshalJSON encodes the flit, carrying its Payload message through the message
// codec so the concrete type can be restored on load. Flits are held in port
// buffers and in switch State, both of which are checkpointed, so the polymorphic
// Payload must round-trip rather than decode to a bare interface.
func (f Flit) MarshalJSON() ([]byte, error) {
	payload, err := messaging.EncodeMsg(f.Payload)
	if err != nil {
		return nil, err
	}

	return json.Marshal(flitJSON{
		Hop:          f.MsgMeta,
		SeqID:        f.SeqID,
		NumFlitInMsg: f.NumFlitInMsg,
		Msg:          f.Msg,
		Payload:      payload,
	})
}

// UnmarshalJSON restores a flit, decoding its Payload message through the
// message codec.
func (f *Flit) UnmarshalJSON(data []byte) error {
	var dto flitJSON
	if err := json.Unmarshal(data, &dto); err != nil {
		return err
	}

	payload, err := messaging.DecodeMsg(dto.Payload)
	if err != nil {
		return err
	}

	f.MsgMeta = dto.Hop
	f.SeqID = dto.SeqID
	f.NumFlitInMsg = dto.NumFlitInMsg
	f.Msg = dto.Msg
	f.Payload = payload

	return nil
}
