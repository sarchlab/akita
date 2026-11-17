package pagetablemmu

import (
	"fmt"
	"math"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/sim/timing"
)

// DefaultSpec returns illustrative four-level geometry and pipeline capacities.
// Supply a memory route, fault handler and reserved pool for the target system.
func DefaultSpec() Spec {
	return Spec{
		Freq:               timing.GHz,
		Log2PageSize:       12,
		VABits:             48,
		Banks:              []BankSpec{{NumSets: 4, NumWays: 2}, {NumSets: 8, NumWays: 2}, {NumSets: 16, NumWays: 4}},
		NumWalkEntries:     16,
		MaxWaitersPerEntry: 8,
		AdmissionWidth:     2,
		MemoryWidth:        1,
		LookupWidth:        1,
		FillWidth:          1,
		LookupLatency:      2,
		FilterWidth:        1,
		FilterLatency:      1,
		FilterMaxKicks:     4,
	}
}

func (s Spec) validate() error {
	if s.Freq == 0 || s.Freq.Period() == 0 {
		return fmt.Errorf("invalid clock frequency")
	}
	if err := s.validateGeometry(); err != nil {
		return err
	}
	if s.NumWalkEntries < 1 || s.MaxWaitersPerEntry < 1 || s.AdmissionWidth < 1 || s.MemoryWidth < 1 {
		return fmt.Errorf("walk capacities and issue widths must be positive")
	}
	if s.LookupWidth < 1 || s.FillWidth < 1 || s.LookupLatency < 1 {
		return fmt.Errorf("invalid STC timing")
	}
	for _, b := range s.Banks {
		if b.NumSets < 0 || b.NumWays < 0 || (b.NumSets == 0) != (b.NumWays == 0) {
			return fmt.Errorf("invalid STC bank geometry")
		}
	}
	if err := s.validateFilter(); err != nil {
		return err
	}
	if s.PoolNodes == 0 || s.PoolBase%nodeBytes != 0 || s.PoolNodes-1 > (math.MaxUint64-s.PoolBase)/nodeBytes {
		return fmt.Errorf("invalid reserved node pool")
	}
	if s.FaultHandler == "" {
		return fmt.Errorf("a fault handler is required")
	}
	if err := s.Memory.Validate(); err != nil {
		return err
	}
	m := s.Memory
	if len(m.Destinations) > 1 && m.InterleavingSize%pteBytes != 0 ||
		m.RangeHigh != 0 && (m.RangeLow%pteBytes != 0 || m.RangeHigh%pteBytes != 0) {
		return fmt.Errorf("memory routing boundaries must be PTE-aligned")
	}
	return nil
}

func (s Spec) validateFilter() error {
	if s.FilterBuckets < 0 || s.FilterBuckets&(s.FilterBuckets-1) != 0 {
		return fmt.Errorf("filter bucket count must be zero or a power of two")
	}
	if s.FilterBuckets > 0 && (s.FilterWidth < 1 || s.FilterLatency < 1 || s.FilterMaxKicks < 0) {
		return fmt.Errorf("invalid SRAM filter timing or kick limit")
	}
	return nil
}

func (s Spec) validateGeometry() error {
	if s.VABits > 64 || s.Log2PageSize >= s.VABits || len(s.Banks) >= maxLevels {
		return fmt.Errorf("invalid virtual address geometry")
	}
	vpnBits := s.VABits - s.Log2PageSize
	if vpnBits > 9*uint(len(s.Banks)+1) {
		return fmt.Errorf("VPN exceeds radix address capacity")
	}
	return nil
}

func (s *state) Initialize(c *Comp) {
	if err := c.Spec.validate(); err != nil {
		panic(fmt.Errorf("%s: %w", c.Name(), err))
	}
	s.Roots = make(map[vm.PID]uint64)
	s.Owners = make(map[uint64]vm.PID)
	s.Walks = make([]walk, c.Spec.NumWalkEntries)
	for _, spec := range c.Spec.Banks {
		s.Banks = append(s.Banks, make(prefixBank, spec.NumSets))
	}
	s.Filter = make([]fingerprint, c.Spec.FilterBuckets)
}

func (m *middlewares) Initialize(c *Comp) {
	m.Control = &controlMiddleware{c}
	m.Walk = &walkMiddleware{c}
	m.Memory = &memoryMiddleware{c}
}
