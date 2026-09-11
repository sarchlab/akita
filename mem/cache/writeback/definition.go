package writeback

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the WriteBackCache component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "WriteBackCache",
	DefaultSpec: Spec{
		Freq:                1 * timing.GHz,
		NumReqPerCycle:      1,
		Log2BlockSize:       6,
		BankLatency:         10,
		WayAssociativity:    4,
		NumBanks:            1,
		NumMSHREntry:        16,
		TotalByteSize:       512 * mem.KB,
		WriteBufferCapacity: 1024,
		MaxInflightFetch:    128,
		MaxInflightEviction: 128,
		InterleavingSize:    4096,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		{Name: "Bottom", Roles: []*messaging.Role{memprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
