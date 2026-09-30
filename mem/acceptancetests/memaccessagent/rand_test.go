package memaccessagent

import (
	"bytes"
	"math/rand"
	"testing"

	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
)

// TestRandStateMatchesMathRand checks that the agent's random source draws
// the same values as a math/rand generator with the same seed, so a seeded
// agent keeps the access stream WithRandSeed used to give it.
func TestRandStateMatchesMathRand(t *testing.T) {
	const seed = 42

	want := rand.New(rand.NewSource(seed))
	got := newRandState(seed)

	for i := range 1000 {
		switch i % 3 {
		case 0:
			if g, w := got.Uint64(), want.Uint64(); g != w {
				t.Fatalf("draw %d: Uint64 %d, want %d", i, g, w)
			}
		case 1:
			if g, w := got.Uint32(), want.Uint32(); g != w {
				t.Fatalf("draw %d: Uint32 %d, want %d", i, g, w)
			}
		default:
			if g, w := got.Float64(), want.Float64(); g != w {
				t.Fatalf("draw %d: Float64 %v, want %v", i, g, w)
			}
		}
	}
}

func buildAgent() *Comp {
	return Definition.Builder().
		WithSimulation(modeling.NewStandaloneSimulation(timing.NewSerialEngine())).
		WithResources(Resources{LowModule: messaging.NewPort(nil, 1, 1, "Mem.Top")}).
		WithPorts(Ports{Mem: messaging.NewPort(nil, 4, 4, "Agent.Mem")}).
		Build("Agent")
}

// TestRandStateSurvivesCheckpoint checks that an agent restored from a
// checkpoint continues the random stream where the saved agent left off.
func TestRandStateSurvivesCheckpoint(t *testing.T) {
	saved := buildAgent()
	for range 10 {
		saved.State.RNG.Uint64()
	}

	var checkpoint bytes.Buffer
	if err := saved.SaveCheckpoint(&checkpoint); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	restored := buildAgent()
	if err := restored.LoadCheckpoint(&checkpoint); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	for i := range 10 {
		if g, w := restored.State.RNG.Uint64(), saved.State.RNG.Uint64(); g != w {
			t.Fatalf("draw %d after restore: %d, want %d", i, g, w)
		}
	}
}
