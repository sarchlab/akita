package gmmu

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the GMMU component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "GMMU",
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		MaxRequestsInFlight: 16,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
		{Name: "Bottom", Roles: []*messaging.Role{vmprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
