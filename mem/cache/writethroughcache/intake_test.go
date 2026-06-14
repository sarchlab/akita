package writethroughcache

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Intake allocTransaction", func() {
	var (
		s    *intake
		next *State
	)

	BeforeEach(func() {
		s = &intake{}
		next = &State{}
	})

	It("should append when no Removed slot is available", func() {
		idx0 := s.allocTransaction(next, transactionState{ID: 1})
		idx1 := s.allocTransaction(next, transactionState{ID: 2})

		Expect(idx0).To(Equal(0))
		Expect(idx1).To(Equal(1))
		Expect(next.Transactions).To(HaveLen(2))
	})

	It("should reuse a Removed slot instead of growing the slice", func() {
		s.allocTransaction(next, transactionState{ID: 1})
		idx1 := s.allocTransaction(next, transactionState{ID: 2})
		s.allocTransaction(next, transactionState{ID: 3})

		// Complete the middle transaction.
		next.Transactions[idx1].Removed = true

		reusedIdx := s.allocTransaction(next, transactionState{ID: 4})

		Expect(reusedIdx).To(Equal(idx1))
		Expect(next.Transactions).To(HaveLen(3))
		Expect(next.Transactions[reusedIdx].ID).To(Equal(uint64(4)))
		Expect(next.Transactions[reusedIdx].Removed).To(BeFalse())
	})

	It("should keep the slice bounded by the active transaction count", func() {
		// Repeatedly allocate then complete a transaction. With slot reuse the
		// slice must never grow beyond the peak number of active transactions
		// (one here), regardless of how many requests are processed.
		for i := 0; i < 1000; i++ {
			idx := s.allocTransaction(next, transactionState{ID: uint64(i)})
			next.Transactions[idx].Removed = true
		}

		Expect(next.Transactions).To(HaveLen(1))
	})
})
