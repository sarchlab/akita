package writethroughcache

import (
	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the WriteThroughCache component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "WriteThroughCache",
	DefaultSpec: Spec{
		Freq:                  1 * timing.GHz,
		NumReqPerCycle:        4,
		Log2BlockSize:         6,
		BankLatency:           20,
		WayAssociativity:      4,
		MaxNumConcurrentTrans: 16,
		NumBanks:              1,
		NumMSHREntry:          4,
		TotalByteSize:         4 * mem.KB,
		DirLatency:            2,
		InterleavingSize:      4096,
		WritePolicyType:       "write-around",
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		{Name: "Bottom", Roles: []*messaging.Role{memprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
