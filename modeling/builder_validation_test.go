package modeling

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/timing"
)

// TestBuild_PanicsOnUncheckpointableState confirms that a component whose State
// would silently lose data across a checkpoint fails loudly at construction.
func TestBuild_PanicsOnUncheckpointableState(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected Build to panic on a State that serializes as {}")
		}
		if !strings.Contains(fmt.Sprint(r), "cannot be checkpointed") {
			t.Fatalf("panic = %v, want mention of 'cannot be checkpointed'", r)
		}
	}()

	NewBuilder[None, hidden, None]().
		WithEngine(timing.NewSerialEngine()).
		Build("BadComp")
}

// TestBuild_AllowsCheckpointableState confirms a normal State builds fine.
func TestBuild_AllowsCheckpointableState(t *testing.T) {
	NewBuilder[None, normalState, None]().
		WithEngine(timing.NewSerialEngine()).
		Build("GoodComp")
}
