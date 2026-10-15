package mem

import (
	"fmt"

	"github.com/sarchlab/akita/v5/sim/messaging"
)

// AddressToPortMapper locates the destination that serves an address.
type AddressToPortMapper interface {
	Find(address uint64) messaging.RemotePort
}

// AddressMapper is immutable address-routing configuration. A single destination
// receives every in-range address; multiple destinations repeat in address order.
// Find requires a configuration that has passed Validate.
type AddressMapper struct {
	Destinations []messaging.RemotePort `json:"destinations"`
	// InterleavingSize is the block size; required with multiple destinations.
	InterleavingSize uint64 `json:"interleaving_size"`
	// RangeLow is inclusive. RangeHigh is exclusive; zero disables filtering.
	RangeLow  uint64 `json:"range_low"`
	RangeHigh uint64 `json:"range_high"`
	// Fallback receives addresses outside a configured range.
	Fallback messaging.RemotePort `json:"fallback"`
}

var _ AddressToPortMapper = AddressMapper{}

// Find selects a destination without translating the address. Interleaving uses
// the absolute address, even when RangeLow is nonzero.
func (m AddressMapper) Find(address uint64) messaging.RemotePort {
	if m.RangeHigh != 0 && (address < m.RangeLow || address >= m.RangeHigh) {
		return m.Fallback
	}
	if len(m.Destinations) == 1 {
		return m.Destinations[0]
	}
	return m.Destinations[(address/m.InterleavingSize)%uint64(len(m.Destinations))]
}

// Validate checks configuration. Component initializers call it before routing.
func (m AddressMapper) Validate() error {
	if len(m.Destinations) == 0 {
		return fmt.Errorf("address mapper: at least one destination is required")
	}
	for i, dst := range m.Destinations {
		if dst == "" {
			return fmt.Errorf("address mapper: destination %d is empty", i)
		}
	}
	if len(m.Destinations) > 1 && m.InterleavingSize == 0 {
		return fmt.Errorf("address mapper: multiple destinations require a positive InterleavingSize")
	}
	if m.RangeHigh != 0 {
		if m.RangeHigh <= m.RangeLow {
			return fmt.Errorf("address mapper: RangeHigh must exceed RangeLow")
		}
		if m.Fallback == "" {
			return fmt.Errorf("address mapper: a configured range requires Fallback")
		}
	}
	return nil
}
