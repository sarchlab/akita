package writeback

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("State allocTransaction", func() {
	var state *State

	BeforeEach(func() {
		state = &State{}
	})

	It("should append when no Removed slot is available", func() {
		idx0 := state.allocTransaction(transactionState{ID: 1})
		idx1 := state.allocTransaction(transactionState{ID: 2})

		Expect(idx0).To(Equal(0))
		Expect(idx1).To(Equal(1))
		Expect(state.Transactions).To(HaveLen(2))
	})

	It("should reuse a Removed slot instead of growing the slice", func() {
		state.allocTransaction(transactionState{ID: 1})
		idx1 := state.allocTransaction(transactionState{ID: 2})
		state.allocTransaction(transactionState{ID: 3})

		state.Transactions[idx1].Removed = true

		reusedIdx := state.allocTransaction(transactionState{ID: 4})

		Expect(reusedIdx).To(Equal(idx1))
		Expect(state.Transactions).To(HaveLen(3))
		Expect(state.Transactions[reusedIdx].ID).To(Equal(uint64(4)))
		Expect(state.Transactions[reusedIdx].Removed).To(BeFalse())
	})

	It("should keep the slice bounded by the active transaction count", func() {
		for i := 0; i < 1000; i++ {
			idx := state.allocTransaction(transactionState{ID: uint64(i)})
			state.Transactions[idx].Removed = true
		}

		Expect(state.Transactions).To(HaveLen(1))
	})

	It("should not reuse the in-flight MSHR owner slot even if Removed", func() {
		// The MSHR stage owns ProcessingMSHREntryIdx across ticks, draining its
		// waiter list. That owner is commonly marked Removed before the list is
		// drained; reusing its slot would clobber MSHRTransactionIndices and
		// strand the remaining coalesced requests.
		ownerIdx := state.allocTransaction(transactionState{
			ID:                     1,
			MSHRTransactionIndices: []int{0, 2},
		})
		state.allocTransaction(transactionState{ID: 2})

		state.Transactions[ownerIdx].Removed = true
		state.HasProcessingMSHREntry = true
		state.ProcessingMSHREntryIdx = ownerIdx

		newIdx := state.allocTransaction(transactionState{ID: 3})

		Expect(newIdx).NotTo(Equal(ownerIdx))
		Expect(state.Transactions[ownerIdx].ID).To(Equal(uint64(1)))
		Expect(state.Transactions[ownerIdx].MSHRTransactionIndices).
			To(Equal([]int{0, 2}))

		// Once the stage releases the owner, its slot becomes reusable.
		state.HasProcessingMSHREntry = false
		reusedIdx := state.allocTransaction(transactionState{ID: 4})
		Expect(reusedIdx).To(Equal(ownerIdx))
	})
})
