package endpoint

import "github.com/sarchlab/akita/v5/messaging"

// AssembledMsg is what an endpoint delivers to a device port in place of the
// original message. The network is a traffic-only model: the endpoint strips
// an outgoing message down to its metadata, carries the metadata in flits,
// and reassembles it at the far end. Receivers under this model only ever see
// the metadata. Bare MsgMeta may not travel as a message (it is the
// envelope), so the reassembled metadata is delivered in this concrete
// wrapper.
type AssembledMsg struct {
	messaging.MsgMeta
}

// Protocol is the endpoint delivery protocol: the endpoint delivers
// reassembled messages to device ports. Defining the protocol registers the
// wrapper type with the checkpoint codec.
var (
	Protocol = messaging.DefineProtocol("endpoint",
		messaging.RoleDef{Name: "delivery",
			Sends: []messaging.Msg{AssembledMsg{}}},
	)
	Delivery = Protocol.Role("delivery")
)
