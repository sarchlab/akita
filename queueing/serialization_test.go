package queueing

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestBufferJSONRoundTrip(t *testing.T) {
	b := NewBuffer[int]("buf", 4)
	b.PushTyped(10)
	b.PushTyped(20)
	b.PushTyped(30)

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// The contents must actually be serialized (the old behavior produced "{}").
	if !strings.Contains(string(data), "30") {
		t.Fatalf("elements were not serialized: %s", data)
	}

	var got Buffer[int]
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if got.Name() != "buf" || got.Capacity() != 4 || got.Size() != 3 {
		t.Fatalf("name/cap/size = %q/%d/%d, want buf/4/3",
			got.Name(), got.Capacity(), got.Size())
	}
	for _, want := range []int{10, 20, 30} { // FIFO order preserved
		if got := got.Pop(); got != want {
			t.Fatalf("Pop = %d, want %d", got, want)
		}
	}
}

func TestPipelineJSONRoundTrip(t *testing.T) {
	p := NewPipeline[int](2, 3)
	p.AcceptWithDelay(1, 2) // lands at stage 0 with a 2-cycle dwell
	p.Accept(2)
	p.Tick(&nopSink[int]{}) // advance so stages/cycle-left are non-trivial

	data, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(data), `"num_stages":3`) {
		t.Fatalf("geometry not serialized: %s", data)
	}

	var got Pipeline[int]
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if !reflect.DeepEqual(got.Stages(), p.Stages()) {
		t.Fatalf("stages mismatch:\n got %+v\nwant %+v", got.Stages(), p.Stages())
	}

	// Draining both from the restored state yields identical output only if
	// width, numStages, stage, lane, and cycle-left all survived.
	if a, b := drainPipeline(&p), drainPipeline(&got); !reflect.DeepEqual(a, b) {
		t.Fatalf("drained output mismatch: %v vs %v", a, b)
	}
}

type nopSink[T any] struct{}

func (nopSink[T]) CanPush() bool { return false }
func (nopSink[T]) PushTyped(T)   {}

func drainPipeline(p *Pipeline[int]) []int {
	sink := NewBuffer[int]("sink", 1024)
	out := []int{}
	for i := 0; i < 1000 && len(p.Stages()) > 0; i++ {
		p.Tick(&sink)
		for sink.Size() > 0 {
			out = append(out, sink.Pop())
		}
	}
	return out
}
