package queueing

import (
	"testing"
)

type testItem struct{ id string }

func (t testItem) TaskID() string { return t.id }

func makePipeline(numStage, width int) (Pipeline, Buffer) {
	buf := NewBuffer("TestBuf", 64)
	p := MakeBuilder().
		WithNumStage(numStage).
		WithCyclePerStage(1).
		WithPipelineWidth(width).
		WithPostPipelineBuffer(buf).
		Build("TestPipeline")
	return p, buf
}

func TestSnapshotPipelineEmpty(t *testing.T) {
	p, _ := makePipeline(3, 1)

	snaps := SnapshotPipeline(p)
	if len(snaps) != 0 {
		t.Fatalf("expected 0 snapshots, got %d", len(snaps))
	}
}

func TestSnapshotPipelineWithItems(t *testing.T) {
	p, _ := makePipeline(3, 1)

	p.Accept(testItem{id: "a"})
	p.Tick() // a moves from stage 0 to stage 1

	p.Accept(testItem{id: "b"}) // b enters stage 0

	snaps := SnapshotPipeline(p)
	if len(snaps) != 2 {
		t.Fatalf("expected 2 snapshots, got %d", len(snaps))
	}

	// b should be in stage 0, a in stage 1
	found := map[string]PipelineStageSnapshot{}
	for _, s := range snaps {
		found[s.Elem.TaskID()] = s
	}

	if found["b"].Stage != 0 {
		t.Errorf("expected b at stage 0, got %d", found["b"].Stage)
	}

	if found["a"].Stage != 1 {
		t.Errorf("expected a at stage 1, got %d", found["a"].Stage)
	}
}

func TestRestorePipeline(t *testing.T) {
	p, buf := makePipeline(3, 1)

	p.Accept(testItem{id: "a"})
	p.Tick() // a at stage 1

	snaps := SnapshotPipeline(p)
	p.Clear()

	// Verify cleared
	if len(SnapshotPipeline(p)) != 0 {
		t.Fatal("pipeline should be empty after Clear")
	}

	RestorePipeline(p, snaps)
	restored := SnapshotPipeline(p)

	if len(restored) != 1 {
		t.Fatalf("expected 1 snapshot after restore, got %d", len(restored))
	}

	if restored[0].Elem.TaskID() != "a" {
		t.Errorf("expected elem 'a', got '%s'", restored[0].Elem.TaskID())
	}

	// Verify pipeline still works: tick until item reaches post-pipeline buffer
	for i := 0; i < 10; i++ {
		p.Tick()
	}

	popped := buf.Pop()
	if popped == nil {
		t.Fatal("expected item in post-pipeline buffer after ticking")
	}

	if popped.(testItem).id != "a" {
		t.Errorf("expected 'a', got '%s'", popped.(testItem).id)
	}
}

func TestSnapshotBufferEmpty(t *testing.T) {
	buf := NewBuffer("Buf", 8)

	elems := SnapshotBuffer(buf)
	if len(elems) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(elems))
	}
}

func TestSnapshotBufferWithItems(t *testing.T) {
	buf := NewBuffer("Buf", 8)
	buf.Push("x")
	buf.Push("y")
	buf.Push("z")

	elems := SnapshotBuffer(buf)
	if len(elems) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(elems))
	}

	if elems[0] != "x" || elems[1] != "y" || elems[2] != "z" {
		t.Errorf("unexpected order: %v", elems)
	}
}

func TestRestoreBuffer(t *testing.T) {
	buf := NewBuffer("Buf", 8)
	buf.Push("a")
	buf.Push("b")

	elems := SnapshotBuffer(buf)
	buf.Clear()

	if buf.Size() != 0 {
		t.Fatal("buffer should be empty after Clear")
	}

	RestoreBuffer(buf, elems)

	if buf.Size() != 2 {
		t.Fatalf("expected size 2, got %d", buf.Size())
	}

	if buf.Pop() != "a" {
		t.Error("expected 'a' first")
	}

	if buf.Pop() != "b" {
		t.Error("expected 'b' second")
	}
}
