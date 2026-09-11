package datamover

import (
	"github.com/sarchlab/akita/v5/mem/datamoverprotocol"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the StreamingDataMover component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name: "StreamingDataMover",
	DefaultSpec: Spec{
		Freq: 1 * timing.GHz,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{datamoverprotocol.Responder}},
		{Name: "Inside", Roles: []*messaging.Role{memprotocol.Requester}},
		{Name: "Outside", Roles: []*messaging.Role{memprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
