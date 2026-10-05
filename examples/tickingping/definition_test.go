package tickingping

import (
	"testing"

	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
)

// TestDefinitionMatchesSource asserts that the inspector's static view of
// this package equals the runtime Definition.
func TestDefinitionMatchesSource(t *testing.T) {
	modelingtest.CheckTicking(t, Definition)
}
