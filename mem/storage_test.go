package mem

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStorage(t *testing.T) {
	t.Run("should read and write in single unit", func(t *testing.T) {
		storage := NewStorage(4096)
		storage.Write(0, []byte{1, 2, 3, 4})

		res := storage.Read(0, 2)
		require.Equal(t, []byte{1, 2}, res)

		res = storage.Read(1, 2)
		require.Equal(t, []byte{2, 3}, res)
	})

	t.Run("should read and write across units", func(t *testing.T) {
		storage := NewStorage(8192)
		storage.Write(4094, []byte{1, 2, 3, 4})

		res := storage.Read(4094, 4)
		require.Equal(t, []byte{1, 2, 3, 4}, res)
	})

	t.Run("should panic if accessing over the capacity", func(t *testing.T) {
		storage := NewStorage(4096)
		require.PanicsWithValue(
			t,
			"accessing physical address beyond the storage capacity",
			func() { storage.Write(4097, []byte{1}) },
		)
		require.PanicsWithValue(
			t,
			"accessing physical address beyond the storage capacity",
			func() { storage.Read(4097, 1) },
		)
	})
}
