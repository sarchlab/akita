package dram

import (
	"github.com/sarchlab/akita/v5/modeling/ticking"
	"github.com/sarchlab/akita/v5/timing"
)

// Definition declares the DRAM controller, a ticking component: its default
// configuration and its behavior. Its ports and middlewares are the fields of
// Ports and Middlewares. The system builder builds an instance with
// Definition.Builder()...Build(name); tooling reads the same declaration
// statically.
//
// Strategy and behavior selection is by configuration, not by injecting
// objects: the scheduler and address mapper are chosen by the Spec.Scheduler /
// Spec.AddrMapper registry keys, the row policy by Spec.PagePolicy, and
// refresh is a middleware. New strategies/behaviors are added in-tree and
// registered — the model the reference simulators use.
var Definition = ticking.Definition[Spec, State, Resources, Ports, Middlewares]{
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
	NewState:       newState,
	NewMiddlewares: newMiddlewares,
}

// newState returns the State of a freshly built controller: empty queues and
// every bank closed.
func newState(c *Comp) State {
	spec := c.Spec()

	return State{
		SubTransQueue: subTransQueueState{
			Entries: []subTransRef{},
		},
		CommandQueues: commandQueueState{
			NumQueues: spec.NumChannel * spec.NumRank,
			Entries:   []queueEntry{},
		},
		BankStates: initBankStatesFlat(
			spec.NumRank, spec.NumBankGroup, spec.NumBank),
	}
}

// newMiddlewares checks the configuration and creates the middlewares. The
// immutable values derived from the Spec — the inter-command timing tables,
// the command completion delays, and the controller strategies with their
// address mapping — are computed once here and held by the bank-tick
// middleware.
func newMiddlewares(c *Comp) Middlewares {
	spec := c.Spec()
	spec.mustBeSupported()

	if c.Resources().Storage == nil {
		panic("dram: Resources.Storage is required")
	}

	return Middlewares{
		Ctrl:    &ctrlMiddleware{comp: c},
		Respond: &respondMW{comp: c},
		Refresh: &refreshMiddleware{comp: c},
		BankTick: &bankTickMW{
			comp:      c,
			timing:    generateTiming(&spec),
			cmdCycles: buildCmdCycles(&spec),
			ctrl:      newController(&spec),
		},
		ParseTop: &parseTopMW{comp: c},
	}
}
