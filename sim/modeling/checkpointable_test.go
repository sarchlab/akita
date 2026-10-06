package modeling

import (
	"fmt"
	"strings"
	"testing"
)

// TestMustBeCheckpointable_PanicsOnUncheckpointableState confirms that a
// component whose State would silently lose data across a checkpoint fails
// loudly at construction. Every model's Build runs this check.
func TestMustBeCheckpointable_PanicsOnUncheckpointableState(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected a panic on a State that serializes as {}")
		}
		if !strings.Contains(fmt.Sprint(r), "cannot be checkpointed") {
			t.Fatalf("panic = %v, want mention of 'cannot be checkpointed'", r)
		}
	}()

	MustBeCheckpointable[None, hidden]("BadComp", None{})
}

// TestMustBeCheckpointable_AllowsCheckpointableState confirms a normal State
// passes.
func TestMustBeCheckpointable_AllowsCheckpointableState(t *testing.T) {
	MustBeCheckpointable[None, normalState]("GoodComp", None{})
}
