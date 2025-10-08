package state

import "testing"

type counterState struct {
	Counter int
}

type pointerState struct {
	Values []int
}

func TestManagerStageAndCommit(t *testing.T) {
	m := NewManager()

	if err := m.Register("counter", &counterState{Counter: 1}); err != nil {
		t.Fatalf("register counter: %v", err)
	}

	staged, err := m.Stage("counter")
	if err != nil {
		t.Fatalf("stage counter: %v", err)
	}

	counter := staged.(*counterState)
	counter.Counter++

	if err := m.Commit("counter"); err != nil {
		t.Fatalf("commit counter: %v", err)
	}

	loaded, err := m.Load("counter")
	if err != nil {
		t.Fatalf("load counter: %v", err)
	}

	if loaded.(*counterState).Counter != 2 {
		t.Fatalf("expected counter to be 2, got %d", loaded.(*counterState).Counter)
	}

	if err := m.Register("counter", &counterState{}); err == nil {
		t.Fatalf("expected duplicate key error")
	}
}

func TestManagerCommitAllAndDiscard(t *testing.T) {
	m := NewManager()

	if err := m.Register("ptr", &pointerState{Values: []int{1, 2}}); err != nil {
		t.Fatalf("register ptr: %v", err)
	}

	staged, err := m.Stage("ptr")
	if err != nil {
		t.Fatalf("stage ptr: %v", err)
	}

	ptr := staged.(*pointerState)
	ptr.Values = append(ptr.Values, 3)

	m.DiscardAll()

	loaded, err := m.Load("ptr")
	if err != nil {
		t.Fatalf("load ptr: %v", err)
	}

	if len(loaded.(*pointerState).Values) != 2 {
		t.Fatalf("discard should have removed staged update")
	}

	staged, err = m.Stage("ptr")
	if err != nil {
		t.Fatalf("stage ptr second time: %v", err)
	}
	ptr = staged.(*pointerState)
	ptr.Values = append(ptr.Values, 4)

	m.CommitAll()

	loaded, err = m.Load("ptr")
	if err != nil {
		t.Fatalf("load ptr after commit: %v", err)
	}

	if got := loaded.(*pointerState).Values; len(got) != 3 || got[2] != 4 {
		t.Fatalf("commit all did not apply staged update, got %v", got)
	}
}

func TestStageReturnsSharedValueWithinRound(t *testing.T) {
	m := NewManager()

	type buffer struct {
		Items []int
	}

	if err := m.Register("buf", &buffer{Items: []int{1}}); err != nil {
		t.Fatalf("register buf: %v", err)
	}

	stagedAAny, err := m.Stage("buf")
	if err != nil {
		t.Fatalf("stage buf first time: %v", err)
	}
	stagedA := stagedAAny.(*buffer)
	stagedA.Items = append(stagedA.Items, 2)

	stagedBAny, err := m.Stage("buf")
	if err != nil {
		t.Fatalf("stage buf second time: %v", err)
	}
	stagedB := stagedBAny.(*buffer)

	if stagedA != stagedB {
		t.Fatalf("expected staged value to be shared, got %p and %p", stagedA, stagedB)
	}

	if err := m.Commit("buf"); err != nil {
		t.Fatalf("commit buf: %v", err)
	}

	loadedAny, err := m.Load("buf")
	if err != nil {
		t.Fatalf("load buf: %v", err)
	}
	loaded := loadedAny.(*buffer)

	if got := loaded.Items; len(got) != 2 || got[1] != 2 {
		t.Fatalf("shared staged value not applied, got %v", got)
	}
}
