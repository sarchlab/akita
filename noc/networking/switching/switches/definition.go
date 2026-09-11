package switches

import (
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the Switch component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
//
// The switch has a dynamic number of ports, added at configuration time with
// MakeSwitchPortAdder; they live in the "Port" group, addressed "Port[0]",
// "Port[1]", ... The count is implicit in how many links get wired.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, modeling.None]{
	Name: "Switch",
	DefaultSpec: Spec{
		Freq: 1 * timing.GHz,
	},
	PortGroups: []modeling.PortGroupDef{
		{
			Name:     "Port",
			Roles:    []*messaging.Role{packetization.Link},
			MinCount: 1,
		},
	},
})
