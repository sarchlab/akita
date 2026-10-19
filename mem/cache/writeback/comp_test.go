package writeback

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStateAllocTransaction(t *testing.T) {
	var st *state

	setup1 := func(t *testing.T) {
		t.Helper()

		st = &state{}
	}

	t.Run("should append when no Removed slot is available", func(t *testing.T) {
		setup1(t)

		idx0 := st.allocTransaction(transactionState{ID: 1})
		idx1 := st.allocTransaction(transactionState{ID: 2})

		require.Equal(t, 0, idx0)
		require.Equal(t, 1, idx1)
		require.Len(t, st.Transactions, 2)
	})

	t.Run("should reuse a Removed slot instead of growing the slice", func(t *testing.T) {
		setup1(t)

		st.allocTransaction(transactionState{ID: 1})
		idx1 := st.allocTransaction(transactionState{ID: 2})
		st.allocTransaction(transactionState{ID: 3})

		st.Transactions[idx1].Removed = true

		reusedIdx := st.allocTransaction(transactionState{ID: 4})

		require.Equal(t, idx1, reusedIdx)
		require.Len(t, st.Transactions, 3)
		require.Equal(t, uint64(4), st.Transactions[reusedIdx].ID)
		require.False(t, st.Transactions[reusedIdx].Removed)
	})

	t.Run("should keep the slice bounded by the active transaction count", func(t *testing.T) {
		setup1(t)

		for i := 0; i < 1000; i++ {
			idx := st.allocTransaction(transactionState{ID: uint64(i)})
			st.Transactions[idx].Removed = true
		}

		require.Len(t, st.Transactions, 1)
	})

	t.Run("should not reuse the in-flight MSHR owner slot even if Removed", func(t *testing.T) {
		setup1(t)

		// The MSHR stage owns ProcessingMSHREntryIdx across ticks, draining its
		// waiter list. That owner is commonly marked Removed before the list is
		// drained; reusing its slot would clobber MSHRTransactionIndices and
		// strand the remaining coalesced requests.
		ownerIdx := st.allocTransaction(transactionState{
			ID:                     1,
			MSHRTransactionIndices: []int{0, 2},
		})
		st.allocTransaction(transactionState{ID: 2})

		st.Transactions[ownerIdx].Removed = true
		st.HasProcessingMSHREntry = true
		st.ProcessingMSHREntryIdx = ownerIdx

		newIdx := st.allocTransaction(transactionState{ID: 3})

		require.NotEqual(t, ownerIdx, newIdx)
		require.Equal(t, uint64(1), st.Transactions[ownerIdx].ID)
		require.Equal(t, []int{0, 2}, st.Transactions[ownerIdx].MSHRTransactionIndices)

		// Once the stage releases the owner, its slot becomes reusable.
		st.HasProcessingMSHREntry = false
		reusedIdx := st.allocTransaction(transactionState{ID: 4})
		require.Equal(t, ownerIdx, reusedIdx)
	})
}
