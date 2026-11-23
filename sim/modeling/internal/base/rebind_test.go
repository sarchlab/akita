package base

import "testing"

type rebindingState struct{ bound *int }

func (s *rebindingState) AfterCheckpointLoad(c *int) { s.bound = c }

type misspelledRebindState struct{}

func (*misspelledRebindState) AfterCheckpointLoad() {}

type valueRebindState struct{}

func (valueRebindState) AfterCheckpointLoad(*int) {}

type plainState struct{}

func TestRebindStateCallsDefinedHook(t *testing.T) {
	component := 7
	var s rebindingState

	RebindState(&s, &component)

	if s.bound != &component {
		t.Fatal("the hook must receive the component")
	}
}

func TestRebindStateIgnoresStateWithoutHook(t *testing.T) {
	component := 7

	RebindState(&plainState{}, &component)
}

func TestRebindStateRejectsValueReceiver(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a value receiver would rebind a copy of State")
		}
	}()

	component := 7
	RebindState(&valueRebindState{}, &component)
}

func TestRebindStateRejectsWrongSignature(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a misspelled hook must not be skipped silently")
		}
	}()

	component := 7
	RebindState(&misspelledRebindState{}, &component)
}
