// Package localproto is a synthetic component that defines its protocol in
// the same package and references the roles by bare identifiers rather than
// package-qualified selectors. The inspector must resolve both forms.
//
// The protocol reuses memprotocol's message types (a message type may belong
// to more than one protocol), so this package introduces no new Msg types
// for the registration audit to track.
package localproto

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
)

// Protocol is a local protocol carrying memprotocol's messages.
var (
	Protocol = messaging.DefineProtocol("inspect.localproto",
		messaging.RoleDef{Name: "producer",
			Sends: []messaging.Msg{memprotocol.ReadReq{}}},
		messaging.RoleDef{Name: "consumer",
			Sends: []messaging.Msg{memprotocol.DataReadyRsp{}}},
	)
	// Producer sends requests.
	Producer = Protocol.Role("producer")
	// Consumer answers them.
	Consumer = Protocol.Role("consumer")
)

// Spec configures the component.
type Spec struct {
	// Width is the datapath width.
	Width int `json:"width"`
}

// Definition references the local roles by bare identifiers.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name:        "LocalProto",
	DefaultSpec: Spec{Width: 2},
	Ports: []modeling.PortDef{
		{Name: "In", Roles: []*messaging.Role{Consumer}},
		{Name: "Feed", Roles: []*messaging.Role{Producer}},
	},
})
