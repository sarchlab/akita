// Package addrfilter matches cache lines and pages against the addressing of
// a control request.
package addrfilter

import "github.com/sarchlab/akita/v5/mem/memcontrolprotocol"

// Filter selects aligned units of 1<<log2UnitSize bytes (cache lines or
// pages): the units that a range intersects, the units of a list of
// addresses, or every unit when the request names neither.
type Filter struct {
	log2UnitSize uint64
	all          bool
	units        map[uint64]bool
	hasRange     bool
	first, last  uint64
}

// New returns the filter of addresses, or of r when hasRange is set. r must
// then be valid.
func New(
	addresses []uint64,
	hasRange bool,
	r memcontrolprotocol.AddressRange,
	log2UnitSize uint64,
) Filter {
	f := Filter{log2UnitSize: log2UnitSize}

	switch {
	case hasRange:
		f.hasRange = true
		f.first, f.last = r.Bounds(log2UnitSize)
	case len(addresses) == 0:
		f.all = true
	default:
		f.units = make(map[uint64]bool, len(addresses))
		for _, a := range addresses {
			f.units[a>>log2UnitSize<<log2UnitSize] = true
		}
	}

	return f
}

// FromReq returns the filter of a request with valid addressing.
func FromReq(req memcontrolprotocol.Req, log2UnitSize uint64) Filter {
	return New(req.Addresses, req.HasRange, req.Range, log2UnitSize)
}

// Matches reports whether the filter selects the unit that contains addr.
func (f Filter) Matches(addr uint64) bool {
	if f.all {
		return true
	}

	unit := addr >> f.log2UnitSize << f.log2UnitSize
	if f.hasRange {
		return f.first <= unit && unit <= f.last
	}

	return f.units[unit]
}
