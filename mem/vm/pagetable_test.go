package vm

import (
	"testing"

	"github.com/stretchr/testify/require"
)

//nolint:funlen // Keep the existing scenarios and shared fixture lifecycle in one test group.
func TestPageTable(t *testing.T) {
	var (
		pageTable *PageTable
		page      Page
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		page = Page{
			PID:      1,
			PAddr:    0x0,
			VAddr:    0x1000,
			PageSize: 4096,
			Valid:    true,
		}
		pageTable = NewPageTable(12)
	}

	t.Run("should panic when inserting a page that is already exist", func(t *testing.T) {
		setup1(t)

		pageTable.Insert(page)
		require.Panics(t, func() {
			pageTable.Insert(page)
		})
	})

	t.Run("should find page", func(t *testing.T) {
		setup1(t)

		pageTable.Insert(page)
		retPage, found := pageTable.Find(1, 0x1000)
		require.True(t, found)
		require.Equal(t, page, retPage)
	})

	t.Run("should find page if address is not aligned", func(t *testing.T) {
		setup1(t)

		pageTable.Insert(page)
		retPage, found := pageTable.Find(1, 0x1024)
		require.True(t, found)
		require.Equal(t, page, retPage)
	})

	t.Run("should remove page", func(t *testing.T) {
		setup1(t)

		page1 := Page{PID: 1, VAddr: 0x1000, PageSize: 4096, Valid: true}
		pageTable.Insert(page1)
		page2 := Page{PID: 2, VAddr: 0x2000, PageSize: 4096, Valid: true}
		pageTable.Insert(page2)

		pageTable.Remove(2, 0x2000)

		retPage1, found1 := pageTable.Find(1, 0x1000)
		require.True(t, found1)
		require.Equal(t, page1, retPage1)
		_, found2 := pageTable.Find(2, 0x2000)
		require.False(t, found2)
	})
}
