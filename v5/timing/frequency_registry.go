package timing

import (
	"fmt"
	"math"
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
