package simplebankedmemory

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the SimpleBankedMemory component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "SimpleBankedMemory",
	DefaultSpec: Spec{
		Freq:                           1 * timing.GHz,
		NumBanks:                       4,
		BankPipelineWidth:              1,
		BankPipelineDepth:              1,
		StageLatency:                   10,
		PostPipelineBufSize:            1,
		Capacity:                       4 * mem.GB,
		BankSelectorKind:               "interleaved",
		BankSelectorLog2InterleaveSize: 6,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
