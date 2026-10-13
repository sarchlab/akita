package ticking_test

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type eventRecorder struct {
	events []timing.Event
}

func (r *eventRecorder) Handle(e timing.Event) { r.events = append(r.events, e) }

// Both newly scheduled events and restored legacy events must save the exact
// pre-rename JSON, including the concrete type tag and secondary queue placement.
func TestEventLegacyCheckpoint(t *testing.T) {
	const legacy = `{"time":0,"primary":[{"type":"github.com/sarchlab/akita/v5/sim/modeling/ticking.TickEvent","payload":{"id":7,"time":42,"handler_id":"h","secondary":false}}],"secondary":[{"type":"github.com/sarchlab/akita/v5/sim/modeling/ticking.TickEvent","payload":{"id":8,"time":42,"handler_id":"h","secondary":true}}]}` + "\n"

	for _, pointer := range []bool{false, true} {
		name, fixture := "value", legacy
		first, second := ticking.MakeEvent(7, 42, "h"), ticking.MakeEvent(8, 42, "h")
		second.Secondary = true
		want := []timing.Event{first, second}
		if pointer {
			name = "pointer"
			fixture = strings.ReplaceAll(legacy, `"type":"github.com`, `"type":"*github.com`)
			// Pointer events needed explicit registration before the rename too.
			timing.RegisterEvent(&ticking.Event{})
			want = []timing.Event{&first, &second}
		}

		t.Run(name, func(t *testing.T) {
			src := timing.NewSerialEngine()
			src.RegisterHandler("h", &eventRecorder{})
			for _, event := range want {
				src.Schedule(event)
			}
			var saved bytes.Buffer
			if err := src.SaveCheckpoint(&saved); err != nil {
				t.Fatal(err)
			}
			if saved.String() != fixture {
				t.Fatalf("new checkpoint = %s, want legacy JSON %s", saved.String(), fixture)
			}

			dst := timing.NewSerialEngine()
			recorder := &eventRecorder{}
			dst.RegisterHandler("h", recorder)
			if err := dst.LoadCheckpoint(strings.NewReader(fixture)); err != nil {
				t.Fatalf("load legacy checkpoint: %v", err)
			}
			saved.Reset()
			if err := dst.SaveCheckpoint(&saved); err != nil {
				t.Fatal(err)
			}
			if saved.String() != fixture {
				t.Fatalf("restored checkpoint = %s, want legacy JSON %s", saved.String(), fixture)
			}
			if err := dst.Run(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(recorder.events, want) || dst.CurrentTime() != 42 {
				t.Fatalf("legacy dispatch at %d = %#v, want %#v at 42", dst.CurrentTime(), recorder.events, want)
			}
			t.Log("Legacy checkpoint preserved byte-for-byte; primary and secondary ticks dispatched at time 42")
		})
	}
}
