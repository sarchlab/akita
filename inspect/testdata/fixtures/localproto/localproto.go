// Package localproto is a synthetic component that defines its protocol in
// the same package as the component. The inspector must resolve the role tags
// on its ports against that protocol.
//
// The protocol reuses memprotocol's message types (a message type may belong
// to more than one protocol), so this package introduces no new Msg types
// for the registration audit to track.
package localproto

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
)

// Protocol is a local protocol carrying memprotocol's messages.
var Protocol = messaging.DefineProtocol(
	messaging.RoleDef{Name: "producer",
		Sends: []any{memprotocol.ReadReq{}}},
	messaging.RoleDef{Name: "consumer",
		Sends: []any{memprotocol.DataReadyRsp{}}},
)

// Spec configures the component.
type Spec struct {
	// Width is the datapath width.
	Width int `json:"width"`
}

// State is the mutable runtime state.
type State struct{}

// Ports speaks both roles of the local protocol, and Sink speaks
// messaging's any role.
type Ports struct {
	In   messaging.Port `akita:"role=github.com/sarchlab/akita/v5/inspect/testdata/fixtures/localproto.consumer"`
	Feed messaging.Port `akita:"role=github.com/sarchlab/akita/v5/inspect/testdata/fixtures/localproto.producer"`
	Sink messaging.Port `akita:"role=github.com/sarchlab/akita/v5/sim/messaging.any"`
}

// Middlewares holds the component's behavior.
type Middlewares struct{}

// Comp is the component.
type Comp = ticking.Component[Spec, State, modeling.None, Ports, Middlewares]

// Definition declares the component.
var Definition = ticking.Definition[Spec, State, modeling.None, Ports, Middlewares]{
	DefaultSpec:    Spec{Width: 2},
	NewMiddlewares: newMiddlewares,
}

func newMiddlewares(*Comp) Middlewares { return Middlewares{} }
