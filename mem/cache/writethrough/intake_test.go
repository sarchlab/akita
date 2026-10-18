package writethrough

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIntakeAllocTransaction(t *testing.T) {
	var (
		s    *intake
		next *state
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		s = &intake{}
		next = &state{}
	}

	t.Run("should append when no Removed slot is available", func(t *testing.T) {
		setup1(t)

		idx0 := s.allocTransaction(next, transactionState{ID: 1})
		idx1 := s.allocTransaction(next, transactionState{ID: 2})

		require.Equal(t, 0, idx0)
		require.Equal(t, 1, idx1)
		require.Len(t, next.Transactions, 2)
	})

	t.Run("should reuse a Removed slot instead of growing the slice", func(t *testing.T) {
		setup1(t)

		s.allocTransaction(next, transactionState{ID: 1})
		idx1 := s.allocTransaction(next, transactionState{ID: 2})
		s.allocTransaction(next, transactionState{ID: 3})

		// Complete the middle transaction.
		next.Transactions[idx1].Removed = true

		reusedIdx := s.allocTransaction(next, transactionState{ID: 4})

		require.Equal(t, idx1, reusedIdx)
		require.Len(t, next.Transactions, 3)
		require.Equal(t, uint64(4), next.Transactions[reusedIdx].ID)
		require.False(t, next.Transactions[reusedIdx].Removed)
	})

	t.Run("should keep the slice bounded by the active transaction count", func(t *testing.T) {
		setup1(t)

		// Repeatedly allocate then complete a transaction. With slot reuse the
		// slice must never grow beyond the peak number of active transactions
		// (one here), regardless of how many requests are processed.
		for i := 0; i < 1000; i++ {
			idx := s.allocTransaction(next, transactionState{ID: uint64(i)})
			next.Transactions[idx].Removed = true
		}

		require.Len(t, next.Transactions, 1)
	})
}
