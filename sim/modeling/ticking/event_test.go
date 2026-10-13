package ticking_test

import (
	"bytes"
	"fmt"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling/ticking"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type eventRecorder struct {
	events []timing.Event
}

func (r *eventRecorder) Handle(e timing.Event) { r.events = append(r.events, e) }

func TestEventCheckpointRoundTrip(t *testing.T) {
	event := ticking.MakeEvent(7, 42, "h")
	// Values are registered by ticking; pointer events need explicit registration.
	timing.RegisterEvent(&ticking.Event{})

	for _, evt := range []timing.Event{event, &event} {
		t.Run(fmt.Sprintf("%T", evt), func(t *testing.T) {
			src := timing.NewSerialEngine()
			src.RegisterHandler("h", &eventRecorder{})
			src.Schedule(evt)

			var saved bytes.Buffer
			if err := src.SaveCheckpoint(&saved); err != nil {
				t.Fatal(err)
			}
			t.Logf("saved checkpoint: %s", saved.String())

			dst := timing.NewSerialEngine()
			recorder := &eventRecorder{}
			dst.RegisterHandler("h", recorder)
			if err := dst.LoadCheckpoint(&saved); err != nil {
				t.Fatal(err)
			}
			if err := dst.Run(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(recorder.events, []timing.Event{evt}) || dst.CurrentTime() != 42 {
				t.Fatalf("dispatch at %d = %#v, want %#v at 42", dst.CurrentTime(), recorder.events, evt)
			}
		})
	}
}
