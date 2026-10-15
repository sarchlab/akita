package mem

import (
	"math"
	"testing"

	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestAddressMapperRouting(t *testing.T) {
	m := AddressMapper{Destinations: []messaging.RemotePort{"A", "B"}, InterleavingSize: 256}
	require.NoError(t, m.Validate())
	for address, want := range map[uint64]messaging.RemotePort{
		0: "A", 255: "A", 256: "B", 511: "B", 512: "A", 768: "B", math.MaxUint64: "B",
	} {
		require.Equal(t, want, m.Find(address), "address %d", address)
	}
	m.RangeLow = 256
	m.RangeHigh = 1024
	m.Fallback = "Other"
	require.NoError(t, m.Validate())
	for address, want := range map[uint64]messaging.RemotePort{
		255: "Other", 256: "B", 512: "A", 1023: "B", 1024: "Other",
	} {
		require.Equal(t, want, m.Find(address), "address %d", address)
	}
	m.Destinations = m.Destinations[:1]
	m.InterleavingSize = 0
	require.NoError(t, m.Validate())
	require.Equal(t, messaging.RemotePort("A"), m.Find(768))
	m.RangeHigh = 0
	m.Fallback = ""
	require.NoError(t, m.Validate())
	require.Equal(t, messaging.RemotePort("A"), m.Find(math.MaxUint64))
	t.Log("Interleaving selects A,B,A,B across successive 256-byte blocks; " +
		"[256,1024) filters to Other outside both boundaries; " +
		"a single unbounded destination needs no fallback")
}

func TestAddressMapperValidation(t *testing.T) {
	for _, m := range []AddressMapper{
		{}, {Destinations: []messaging.RemotePort{""}},
		{Destinations: []messaging.RemotePort{"A", "B"}},
		{Destinations: []messaging.RemotePort{"A"}, RangeLow: 4, RangeHigh: 4, Fallback: "F"},
		{Destinations: []messaging.RemotePort{"A"}, RangeLow: 4, RangeHigh: 3, Fallback: "F"},
		{Destinations: []messaging.RemotePort{"A"}, RangeHigh: 4},
	} {
		require.Error(t, m.Validate(), "%+v", m)
	}
}
