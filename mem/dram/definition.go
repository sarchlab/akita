package dram

import (
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns the component's default configuration.
func DefaultSpec() Spec {
	return Spec{
		Freq:                 1600 * timing.MHz,
		Protocol:             ProtocolDDR3,
		PagePolicy:           PagePolicyClose,
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
	}
}

// Initialize creates the State of a freshly built controller: empty queues and
// every bank closed.
func (s *state) Initialize(c *Comp) {
	spec := c.Spec

	*s = state{
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

// Initialize checks the configuration and creates the middlewares. The
// immutable values derived from the Spec — the inter-command timing tables,
// the command completion delays, and the controller strategies with their
// address mapping — are computed once here and held by the bank-tick
// middleware.
func (m *middlewares) Initialize(c *Comp) {
	spec := c.Spec
	spec.mustBeSupported()

	if c.Resources.Storage == nil {
		panic("dram: Resources.Storage is required")
	}

	*m = middlewares{
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
