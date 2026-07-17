package memaccessagent

import (
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the MemAccessAgent component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name: "MemAccessAgent",
	DefaultSpec: Spec{
		Freq:       1 * timing.GHz,
		MaxAddress: 1024 * 1024,
		WriteLeft:  1000,
		ReadLeft:   1000,
	},
	Ports: []modeling.PortDef{
		{Name: "Mem", Roles: []*messaging.Role{memprotocol.Requester}},
	},
})
