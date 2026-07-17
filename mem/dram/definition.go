package dram

import (
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the DRAMController component: its default configuration and
// its port topology. The builder consumes it at runtime and tooling reads it
// statically, so it is the single source of truth for both.
var Definition = modeling.DefineComponent(modeling.ComponentDef[Spec, Resources]{
	Name: "DRAMController",
	DefaultSpec: Spec{
		Freq:                 1600 * timing.MHz,
		Protocol:             int(protoDDR3),
		TAL:                  0,
		TCL:                  11,
		TCWL:                 8,
		TRCD:                 11,
		TRP:                  11,
		TRAS:                 28,
		TCCDL:                4,
		TCCDS:                4,
		TRTRS:                1,
		TRTP:                 6,
		TWTRL:                6,
		TWTRS:                6,
		TWR:                  12,
		TPPD:                 0,
		TRRDL:                5,
		TRRDS:                5,
		TRCDRD:               24,
		TRCDWR:               20,
		TREFI:                6240,
		TRFC:                 208,
		TRFCb:                1950,
		TCKESR:               5,
		TXS:                  216,
		BusWidth:             64,
		BurstLength:          8,
		DeviceWidth:          16,
		NumChannel:           1,
		NumRank:              2,
		NumBankGroup:         1,
		NumBank:              8,
		NumRow:               32768,
		NumCol:               1024,
		TransactionQueueSize: 32,
		CommandQueueCapacity: 8,
	},
	Ports: []modeling.PortDef{
		{Name: "Top", Roles: []*messaging.Role{memprotocol.Responder}},
		{Name: "Control", Roles: []*messaging.Role{memcontrolprotocol.Responder}},
	},
})
