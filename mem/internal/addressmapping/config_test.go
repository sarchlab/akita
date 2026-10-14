package addressmapping

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/stretchr/testify/require"
)

func TestMappingModesAndRange(t *testing.T) {
	ports := []messaging.RemotePort{"A.Top", "B.Top"}
	interleaved := Config{Kind: mem.AddressMapperInterleaved, Ports: ports, InterleavingSize: 64,
		UseAddressSpaceLimitation: true, LowAddress: 128, HighAddress: 512, ModuleForOtherAddresses: "Fallback.Top"}
	m := interleaved.Mapper()
	for address, want := range map[uint64]messaging.RemotePort{
		0: "Fallback.Top", 127: "Fallback.Top", 128: "A.Top", 191: "A.Top", 192: "B.Top", 512: "Fallback.Top",
	} {
		require.Equal(t, want, m.Find(address), "address %d", address)
	}
	banked := Config{Kind: mem.AddressMapperBanked, Ports: ports, BankSize: 256}.Mapper()
	require.Equal(t, ports[0], banked.Find(255))
	require.Equal(t, ports[1], banked.Find(256))
	single := Config{Kind: mem.AddressMapperSingle, Ports: ports[:1]}.Mapper()
	ports[0] = "Changed.Top"
	require.Equal(t, messaging.RemotePort("A.Top"), m.Find(128))
	require.Equal(t, messaging.RemotePort("A.Top"), single.Find(999))
}

func TestInvalidMappingConfiguration(t *testing.T) {
	for _, config := range []Config{
		{Kind: "invalid"},
		{Ports: []messaging.RemotePort{"A.Top"}},
		{Kind: mem.AddressMapperSingle, Ports: []messaging.RemotePort{""}},
		{Kind: mem.AddressMapperInterleaved, Ports: []messaging.RemotePort{"A.Top"}},
		{Kind: mem.AddressMapperBanked, Ports: []messaging.RemotePort{"A.Top"}},
		{Kind: mem.AddressMapperInterleaved, Ports: []messaging.RemotePort{"A.Top"}, InterleavingSize: 64,
			UseAddressSpaceLimitation: true, LowAddress: 10, HighAddress: 10},
	} {
		require.Error(t, config.Validate())
	}
	require.Nil(t, Config{Kind: mem.AddressMapperSingle}.Mapper())
}
