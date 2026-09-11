package mmuCache

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the MMUCache component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "MMUCache",
	DefaultSpec: Spec{
		Freq:            1 * timing.GHz,
		NumReqPerCycle:  4,
		NumLevels:       5,
		NumBlocks:       1,
		PageSize:        4096,
		LatencyPerLevel: 100,
		Log2PageSize:    12,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
		{Name: "Bottom", Roles: []*messaging.Role{vmprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
