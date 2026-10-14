// Package addressmapping builds runtime lookup objects from flat Spec fields.
package addressmapping

import (
	"fmt"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

// Config is a temporary view of a component's flat, checkpointed Spec fields.
// Components keep those fields in Spec and the derived mapper in middleware.
type Config struct {
	Kind                      mem.AddressMapperType
	Ports                     []messaging.RemotePort
	InterleavingSize          uint64
	BankSize                  uint64
	UseAddressSpaceLimitation bool
	LowAddress                uint64
	HighAddress               uint64
	ModuleForOtherAddresses   messaging.RemotePort
}

func (c Config) Validate() error {
	switch c.Kind {
	case "":
		if len(c.Ports) != 0 {
			return fmt.Errorf("address mapper kind is required")
		}
	case mem.AddressMapperSingle, mem.AddressMapperInterleaved, mem.AddressMapperBanked:
	default:
		return fmt.Errorf("unknown address mapper type %q", c.Kind)
	}
	if len(c.Ports) == 0 {
		return nil
	}
	for _, port := range c.Ports {
		if port == "" {
			return fmt.Errorf("address mapper destination cannot be empty")
		}
	}
	if c.Kind == mem.AddressMapperInterleaved && c.InterleavingSize == 0 {
		return fmt.Errorf("interleaved mapping requires non-zero Spec.InterleavingSize")
	}
	if c.Kind == mem.AddressMapperBanked && c.BankSize == 0 {
		return fmt.Errorf("banked mapping requires non-zero bank size")
	}
	if c.UseAddressSpaceLimitation && (c.Kind != mem.AddressMapperInterleaved || c.HighAddress <= c.LowAddress) {
		return fmt.Errorf("address range requires interleaved mapping and low < high")
	}
	return nil
}

func (c Config) Mapper() mem.AddressToPortMapper {
	if err := c.Validate(); err != nil {
		panic(err)
	}
	if len(c.Ports) == 0 {
		return nil
	}
	ports := append([]messaging.RemotePort(nil), c.Ports...)
	switch c.Kind {
	case mem.AddressMapperSingle:
		return &mem.SinglePortMapper{Port: ports[0]}
	case mem.AddressMapperInterleaved:
		return &mem.InterleavedAddressPortMapper{LowModules: ports, InterleavingSize: c.InterleavingSize,
			UseAddressSpaceLimitation: c.UseAddressSpaceLimitation, LowAddress: c.LowAddress, HighAddress: c.HighAddress,
			ModuleForOtherAddresses: c.ModuleForOtherAddresses}
	case mem.AddressMapperBanked:
		return &mem.BankedAddressPortMapper{LowModules: ports, BankSize: c.BankSize}
	default:
		panic("unreachable address mapper kind")
	}
}
