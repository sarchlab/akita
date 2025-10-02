package timing

import (
	"errors"
	"math"
	"testing"
)

func TestRegisterFrequencySingleDomain(t *testing.T) {
	registry := NewFrequencyRegistry()

	domain, err := registry.RegisterFrequency(GHz)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if domain == nil {
		t.Fatalf("expected domain, got nil")
	}

	if got, want := domain.FrequencyHz(), GHz; got != want {
		t.Fatalf("frequency mismatch: got %d, want %d", got, want)
	}

	if got, want := domain.Stride(), VTimeInCycle(1); got != want {
		t.Fatalf("stride mismatch: got %d, want %d", got, want)
	}

	// Re-registering the same frequency should return the same domain instance.
	domain2, err := registry.RegisterFrequency(GHz)
	if err != nil {
		t.Fatalf("unexpected error on re-register: %v", err)
	}
	if domain2 != domain {
		t.Fatalf("expected identical domain pointers on re-register")
	}
}

func TestRegisterFrequencyMultipleDomains(t *testing.T) {
	registry := NewFrequencyRegistry()

	slowDomain, err := registry.RegisterFrequency(GHz)
	if err != nil {
		t.Fatalf("unexpected error registering slow domain: %v", err)
	}

	fastFreq := FreqInHz(2500) * MHz // 2.5 GHz
	fastDomain, err := registry.RegisterFrequency(fastFreq)
	if err != nil {
		t.Fatalf("unexpected error registering fast domain: %v", err)
	}

	if got, want := slowDomain.Stride(), VTimeInCycle(5); got != want {
		t.Fatalf("slow domain stride mismatch: got %d, want %d", got, want)
	}

	if got, want := fastDomain.Stride(), VTimeInCycle(2); got != want {
		t.Fatalf("fast domain stride mismatch: got %d, want %d", got, want)
	}

	if got, want := fastDomain.ThisTick(5), VTimeInCycle(6); got != want {
		t.Fatalf("ThisTick mismatch: got %d, want %d", got, want)
	}

	if got, want := fastDomain.NextTick(6), VTimeInCycle(8); got != want {
		t.Fatalf("NextTick mismatch: got %d, want %d", got, want)
	}

	if got, want := slowDomain.NTicksLater(0, VTimeInCycle(3)), VTimeInCycle(15); got != want {
		t.Fatalf("NTicksLater mismatch: got %d, want %d", got, want)
	}

	overflowStart := VTimeInCycle(math.MaxUint64 - 4) // stride is 5
	if got, want := slowDomain.NTicksLater(overflowStart, VTimeInCycle(1)), VTimeInCycle(math.MaxUint64); got != want {
		t.Fatalf("NTicksLater overflow mismatch: got %d, want %d", got, want)
	}
}

func TestSecondsCyclesConversions(t *testing.T) {
	registry := NewFrequencyRegistry()

	if _, err := registry.secondsToCycles(1); !errors.Is(err, ErrNoFrequencyDomains) {
		t.Fatalf("expected ErrNoFrequencyDomains, got %v", err)
	}

	if _, err := registry.RegisterFrequency(GHz); err != nil {
		t.Fatalf("register frequency: %v", err)
	}
	if _, err := registry.RegisterFrequency(FreqInHz(2500) * MHz); err != nil {
		t.Fatalf("register frequency: %v", err)
	}

	cycles, err := registry.secondsToCycles(VTimeInSec(1e-9))
	if err != nil {
		t.Fatalf("SecondsToCycles returned error: %v", err)
	}
	if got, want := cycles, VTimeInCycle(5); got != want {
		t.Fatalf("SecondsToCycles mismatch: got %d, want %d", got, want)
	}

	if secs := registry.cyclesToSeconds(VTimeInCycle(10)); secs != VTimeInSec(2e-9) {
		t.Fatalf("CyclesToSeconds mismatch: got %g, want %g", secs, VTimeInSec(2e-9))
	}

	if _, err := registry.secondsToCycles(VTimeInSec(1.5e-10)); !errors.Is(err, ErrTickPrecisionLoss) {

		t.Fatalf("expected ErrTickPrecisionLoss, got %v", err)
	}
}
