package timing

import (
	"errors"
	"fmt"
	"math"
	"math/bits"
)

// FreqInHz defines frequency in the unit of Hertz (cycles per second).
type FreqInHz uint64

const (
	Hz  = FreqInHz(1)
	kHz = FreqInHz(1000 * Hz)
	MHz = FreqInHz(1000 * kHz)
	GHz = FreqInHz(1000 * MHz)
)

// VTimeInSec expresses time in seconds inside the timing helpers. It should
// only be used for reporting purposes. Internally, the simulator should use
// VTimeInCycle to avoid floating-point precision issues.
type VTimeInSec float64

// VTimeInCycle is the canonical time quantum used by the simulator. All
// timestamps are expressed as multiples of cycles to keep ordering deterministic
// across domains.
type VTimeInCycle uint64

var (
	// ErrZeroFrequency indicates that a domain attempted to register a clock
	// with a zero frequency, which is not meaningful.
	ErrZeroFrequency = errors.New("timing: frequency must be greater than zero")

	// ErrFrequencyOverflow indicates that the derived global frequency exceeds
	// what can be represented in a uint64. This happens when the least common
	// multiple of registered domains does not fit in 64 bits.
	ErrFrequencyOverflow = errors.New("timing: global frequency overflow")

	// ErrNoFrequencyDomains indicates that no domains have been registered yet
	// so conversions between cycles and seconds cannot be performed.
	ErrNoFrequencyDomains = errors.New("timing: no frequency domains registered")

	// ErrTickPrecisionLoss indicates that a conversion from seconds to cycles
	// would require precision beyond the selected cycle resolution.
	ErrTickPrecisionLoss = errors.New(
		"timing: duration is not aligned with cycle resolution",
	)

	// ErrTickOverflow indicates that the computed number of cycles exceeds the
	// representable range of VTimeInCycle (uint64).
	ErrTickOverflow = errors.New("timing: cycle value overflow")
)

// FrequencyRegistry coordinates multiple clock domains by deriving a single
// cycle resolution that preserves deterministic ordering.
type FrequencyRegistry struct {
	global  FreqInHz
	domains map[FreqInHz]*FreqDomain
}

// NewFrequencyRegistry builds an empty registry ready to accept clock domains.
func NewFrequencyRegistry() *FrequencyRegistry {
	return &FrequencyRegistry{
		domains: make(map[FreqInHz]*FreqDomain),
	}
}

// RegisterFrequency adds a clock domain and returns its descriptor.
func (r *FrequencyRegistry) RegisterFrequency(
	freq FreqInHz,
) (*FreqDomain, error) {
	if freq == 0 {
		return nil, ErrZeroFrequency
	}

	if domain, exists := r.domains[freq]; exists {
		return domain, nil
	}

	if r.global == 0 {
		r.global = freq
	} else {
		newGlobal, err := lcmFreq(r.global, freq)
		if err != nil {
			return nil, err
		}
		r.global = newGlobal
	}

	domain := &FreqDomain{
		freq:     freq,
		registry: r,
	}
	r.domains[freq] = domain
	return domain, nil
}
func (r *FrequencyRegistry) cyclesToSeconds(cycles VTimeInCycle) VTimeInSec {
	if r.global == 0 {
		return 0
	}

	return VTimeInSec(float64(cycles) / float64(r.global))
}

func (r *FrequencyRegistry) secondsToCycles(sec VTimeInSec) (VTimeInCycle, error) {
	if r.global == 0 {
		return 0, ErrNoFrequencyDomains
	}
	if sec < 0 {
		return 0, fmt.Errorf(
			"timing: negative durations are not supported: %.12g",
			sec,
		)
	}

	scaled := float64(sec) * float64(r.global)
	rounded := math.Round(scaled)
	tickDuration := 1.0 / float64(r.global)
	if math.Abs(scaled-rounded) > cycleAlignmentTolerance(scaled) {
		return 0, fmt.Errorf(
			"%w: duration %.12g s exceeds cycle %.12g s",
			ErrTickPrecisionLoss,
			sec,
			tickDuration,
		)
	}

	if rounded < 0 || rounded > float64(math.MaxUint64) {
		return 0, ErrTickOverflow
	}

	return VTimeInCycle(rounded), nil
}

const maxCycleValue = VTimeInCycle(math.MaxUint64)

// FreqDomain represents a registered clock domain.
type FreqDomain struct {
	freq     FreqInHz
	registry *FrequencyRegistry
}

// FrequencyHz returns the raw frequency associated with the domain.
func (d *FreqDomain) FrequencyHz() FreqInHz {
	if d == nil {
		return 0
	}
	return d.freq
}

// Stride returns the number of global cycles contained in one domain cycle.
func (d *FreqDomain) Stride() VTimeInCycle {
	return d.stride()
}

// ThisTick aligns the provided global cycle to the earliest domain tick that is
// not earlier than the input.
func (d *FreqDomain) ThisTick(now VTimeInCycle) VTimeInCycle {
	stride := d.stride()
	if stride == 0 {
		return 0
	}

	tick, ok := roundUpToStride(now, stride)
	if !ok {
		return maxCycleValue
	}

	return tick
}

// NextTick advances to the next domain tick strictly after the provided cycle
// count.
func (d *FreqDomain) NextTick(now VTimeInCycle) VTimeInCycle {
	stride := d.stride()
	if stride == 0 {
		return 0
	}

	tick, ok := roundUpToStride(now, stride)
	if !ok {
		return maxCycleValue
	}
	if tick == now {
		next, ok := addCycles(now, stride)
		if !ok {
			return maxCycleValue
		}
		return next
	}

	return tick
}

// NTicksLater advances the provided cycle count by the specified number of
// domain ticks.
func (d *FreqDomain) NTicksLater(now, ticks VTimeInCycle) VTimeInCycle {
	stride := d.stride()
	if stride == 0 {
		return 0
	}
	if ticks == 0 {
		return d.ThisTick(now)
	}

	offset, ok := mulCycles(ticks, stride)
	if !ok {
		return maxCycleValue
	}
	future, ok := addCycles(now, offset)
	if !ok {
		return maxCycleValue
	}
	tick, ok := roundUpToStride(future, stride)
	if !ok {
		return maxCycleValue
	}

	return tick
}

func (d *FreqDomain) stride() VTimeInCycle {
	if d == nil || d.registry == nil {
		return 0
	}
	global := d.registry.global
	freq := d.freq
	if global == 0 || freq == 0 || global%freq != 0 {
		return 0
	}
	return VTimeInCycle(global / freq)
}

func roundUpToStride(value, stride VTimeInCycle) (VTimeInCycle, bool) {
	if stride == 0 {
		return 0, true
	}
	remainder := value % stride
	if remainder == 0 {
		return value, true
	}
	delta := stride - remainder
	return addCycles(value, delta)
}

func addCycles(a, b VTimeInCycle) (VTimeInCycle, bool) {
	if uint64(a) > math.MaxUint64-uint64(b) {
		return maxCycleValue, false
	}
	return a + b, true
}

func mulCycles(a, b VTimeInCycle) (VTimeInCycle, bool) {
	hi, lo := bits.Mul64(uint64(a), uint64(b))
	if hi != 0 {
		return maxCycleValue, false
	}
	return VTimeInCycle(lo), true
}

func cycleAlignmentTolerance(value float64) float64 {
	// The tolerance scales with the magnitude of the value to account for
	// floating-point rounding noise. The constant roughly matches one half of
	// a unit in the last place for 53-bit mantissas.
	const ulpFactor = 1e-9
	v := math.Abs(value)
	if v < 1 {
		return ulpFactor
	}
	return v * ulpFactor
}

func lcmFreq(a, b FreqInHz) (FreqInHz, error) {
	g := gcdFreq(a, b)
	if g == 0 {
		return 0, ErrZeroFrequency
	}

	quotient := uint64(a / g)
	if quotient > math.MaxUint64/uint64(b) {
		return 0, ErrFrequencyOverflow
	}

	return FreqInHz(quotient * uint64(b)), nil
}

func gcdFreq(a, b FreqInHz) FreqInHz {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}
