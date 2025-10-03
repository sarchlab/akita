package idgen_test

import (
	"testing"

	"github.com/sarchlab/akita/v4/v5/idgen"
)

func TestSequentialDeterministic(t *testing.T) {
	g := idgen.New()

	wants := []idgen.ID{1, 2, 3}
	for _, want := range wants {
		if got := g.Generate(); got != want {
			t.Fatalf("Generate() = %d, want %d", got, want)
		}
	}
}

func TestIndependentGenerators(t *testing.T) {
	g1 := idgen.New()
	g2 := idgen.New()

	if g1.Generate() != idgen.ID(1) {
		t.Fatalf("unexpected first id from g1")
	}

	if g2.Generate() != idgen.ID(1) {
		t.Fatalf("unexpected first id from g2")
	}

	if g1.Generate() != idgen.ID(2) {
		t.Fatalf("unexpected second id from g1")
	}
}
