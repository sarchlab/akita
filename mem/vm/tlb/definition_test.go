package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/modeling/modelingtest"
)

// TestDefinitionMatchesSource asserts that the inspector's static view of
// this package equals the runtime Definition, so the two can never drift.
func TestDefinitionMatchesSource(t *testing.T) {
	modelingtest.CheckDefinition(t, Definition,
		"github.com/sarchlab/akita/v5/mem/vm/tlb")
}
