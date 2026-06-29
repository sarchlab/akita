package dram

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// PER_BANK command-queue structure.
//
// Today the controller uses one command queue per rank (PER_RANK): all of a
// rank's banks share a single queue of CommandQueueCapacity entries, so a burst
// to one bank can starve the others. PER_BANK gives each bank its own queue,
// which both references model. These specs capture the PER_BANK behavior and
// are expected to FAIL until it is implemented (getQueueIndex / NumQueues).

func cmdAt(rank, bankGroup, bank uint64) *commandState {
	return &commandState{
		Kind:     int(cmdKindRead),
		Location: location{Rank: rank, BankGroup: bankGroup, Bank: bank},
	}
}

var _ = Describe("PER_BANK command queues", func() {
	It("maps different banks of a rank to different queues (PER_BANK)", func() {
		spec := DefaultSpec()
		spec.QueueStructure = QueueStructurePerBank

		// Two distinct banks in the same rank must land in distinct queues.
		Expect(getQueueIndex(&spec, cmdAt(0, 0, 0))).
			NotTo(Equal(getQueueIndex(&spec, cmdAt(0, 0, 1))))
	})

	It("isolates command-queue capacity per bank (PER_BANK)", func() {
		spec := DefaultSpec()
		spec.QueueStructure = QueueStructurePerBank
		spec.CommandQueueCapacity = 2

		state := &State{
			CommandQueues: commandQueueState{Entries: []queueEntry{}},
		}

		// Fill bank (0,0,0)'s queue to capacity.
		acceptCommand(state, cmdAt(0, 0, 0), &spec)
		acceptCommand(state, cmdAt(0, 0, 0), &spec)

		// A different bank in the same rank still has room — its own queue.
		Expect(canAcceptCommand(state, cmdAt(0, 0, 1), &spec)).To(BeTrue())
	})

	It("builds one command queue per bank (PER_BANK)", func() {
		spec := DDR4Spec
		spec.QueueStructure = QueueStructurePerBank
		spec.TransactionQueueSize = 32
		spec.CommandQueueCapacity = 8

		h := newDramHarness(spec)
		banks := spec.NumRank * spec.NumBankGroup * spec.NumBank
		Expect(h.dram.State.CommandQueues.NumQueues).To(Equal(banks))
	})

	It("groups a rank's banks into one queue under PER_RANK (default)", func() {
		spec := DefaultSpec()
		spec.QueueStructure = QueueStructurePerRank

		// Same rank, different banks share the single per-rank queue.
		Expect(getQueueIndex(&spec, cmdAt(0, 0, 0))).
			To(Equal(getQueueIndex(&spec, cmdAt(0, 0, 1))))
	})
})
