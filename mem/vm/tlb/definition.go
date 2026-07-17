package tlb

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the TLB component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "TLB",
	DefaultSpec: Spec{
		Freq:           1 * timing.GHz,
		NumReqPerCycle: 4,
		NumSets:        1,
		NumWays:        32,
		Log2PageSize:   12,
		PageSize:       4096,
		MSHRSize:       4,
		Latency:        4,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{vmprotocol.Responder}},
		{Name: "Bottom", Roles: []*messaging.Role{vmprotocol.Requester}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
