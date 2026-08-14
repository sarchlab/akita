package idealmemcontroller

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the IdealMemController component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "IdealMemController",
	DefaultSpec: Spec{
		Freq:          1 * timing.GHz,
		Latency:       100,
		Width:         1,
		CacheLineSize: 64,
		Capacity:      4 * mem.GB,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
