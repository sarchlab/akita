package ping

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
)

// TestComponentMatchesSource asserts that the inspector's static view of
// this package equals the runtime component type and DefaultSpec() result.
func TestComponentMatchesSource(t *testing.T) {
	modelingtest.CheckWakeup(t, (*Comp)(nil), DefaultSpec())
}
