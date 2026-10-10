package direct

import (
	"bytes"
	"testing"

	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"go.uber.org/mock/gomock"
)

func TestNewConnectionUsesExplicitFrequency(t *testing.T) {
	ctrl := gomock.NewController(t)
	engine := NewMockEngine(ctrl)
	engine.EXPECT().SetRunGuard(gomock.Any()).AnyTimes()
	engine.EXPECT().RegisterHandler("Link", gomock.Any())
	engine.EXPECT().CurrentTime().Return(timing.VTimeInPicoSec(250))
	engine.EXPECT().Schedule(gomock.Any()).Do(func(evt timing.Event) {
		if evt.Time() != 2000 || !evt.IsSecondary() {
			t.Fatalf("scheduled tick at %d ps, secondary=%v; want 2000 ps, secondary=true",
				evt.Time(), evt.IsSecondary())
		}
		t.Log("500 MHz connection schedules a secondary tick at 2000 ps from 250 ps")
	})

	s := modeling.NewStandaloneSimulation(engine)
	conn := NewConnection("Link", s, 500*timing.MHz)
	conn.NotifySend()
}

func TestNewConnectionCheckpointFrequency(t *testing.T) {
	s := modeling.NewStandaloneSimulation(timing.NewSerialEngine())
	conn := NewConnection("Source", s, 500*timing.MHz)
	conn.State.NextPortID = 3
	var saved bytes.Buffer
	if err := conn.SaveCheckpoint(&saved); err != nil {
		t.Fatal(err)
	}

	same := NewConnection("Same", s, 500*timing.MHz)
	if err := same.LoadCheckpoint(bytes.NewReader(saved.Bytes())); err != nil {
		t.Fatal(err)
	}
	if same.State.NextPortID != 3 {
		t.Fatalf("restored cursor = %d, want 3", same.State.NextPortID)
	}

	different := NewConnection("Different", s, timing.GHz)
	if err := different.LoadCheckpoint(bytes.NewReader(saved.Bytes())); err == nil {
		t.Fatal("checkpoint with a different frequency was accepted")
	}
	t.Log("matching frequency restores the cursor; changed frequency is rejected")
}
