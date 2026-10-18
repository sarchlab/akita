package mem

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestInterleavingAddressConverter(t *testing.T) {
	var (
		converter InterleavingConverter
	)

	setup1 := func(t *testing.T) {
		t.Helper()

		converter.InterleavingSize = 4096
		converter.TotalNumOfElements = 8
		converter.CurrentElementIndex = 1
	}

	t.Run("should convert from external to internal", func(t *testing.T) {
		setup1(t)

		require.Equal(t, uint64(0), converter.ConvertExternalToInternal(4096))
		require.Equal(t, uint64(4096), converter.ConvertExternalToInternal(4096*8+4096))
		require.Equal(t, uint64(8192), converter.ConvertExternalToInternal(4096*8*2+4096))
		require.Equal(t, uint64(8292), converter.ConvertExternalToInternal(4096*8*2+4096+100))
	})

	t.Run("should panic when converting address does not belongs to current "+
		"element", func(t *testing.T) {
		setup1(t)

		require.Panics(t, func() {
			converter.ConvertExternalToInternal(0)
		})
	})
}
