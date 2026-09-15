package mmu

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the MMU component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "MMU",
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		Log2PageSize:        12,
		Latency:             10,
		MaxRequestsInFlight: 16,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
