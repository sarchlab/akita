package memaccessagent_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/idealmemcontroller"
	"github.com/sarchlab/akita/v5/mem/internal/memaccessagent"
	akitasim "github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type countingTracker struct {
	started, inProgress, finished uint64
}

func (p *countingTracker) IncrementInProgress(n uint64)      { p.started += n; p.inProgress += n }
func (p *countingTracker) MoveInProgressToFinished(n uint64) { p.inProgress -= n; p.finished += n }

func buildProgressTestAgent() (*memaccessagent.Comp, timing.Engine) {
	s := akitasim.MakeBuilder().Build()
	engine := s.Engine().(*timing.SerialEngine)
	controller := modeling.MakeBuilder[*idealmemcontroller.Comp](idealmemcontroller.DefaultSpec()).WithSimulation(s).
		WithResources(idealmemcontroller.Resources{Storage: mem.NewStorage(4096)}).
		Build("Mem")

	controller.BindPort("Top", twowaybuffered.NewPort(4, 4))
	controller.BindPort("Control", twowaybuffered.NewPort(4, 4))

	spec := memaccessagent.DefaultSpec()
	spec.MaxAddress, spec.WriteLeft, spec.ReadLeft = 4096, 8, 8
	spec.LowModule = controller.Ports.Top.AsRemote()
	agent := modeling.MakeBuilder[*memaccessagent.Comp](spec).WithSimulation(s).
		Build("Agent")

	agent.BindPort("Mem", twowaybuffered.NewPort(4, 4))

	connection := direct.NewConnection("Conn", s, timing.GHz)
	connection.BindPort(agent.Ports.Mem)
	connection.BindPort(controller.Ports.Top)
	if err := s.Initialize(); err != nil {
		panic(err)
	}

	return agent, engine
}

func TestProgressTrackersObserveCompletedMemoryTraffic(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		agent, engine := buildProgressTestAgent()
		writes, reads := &countingTracker{}, &countingTracker{}
		if enabled {
			memaccessagent.SetProgressTrackers(agent, writes, reads)
		} else {
			memaccessagent.SetProgressTrackers(agent, nil, nil)
		}
		agent.TickLater()
		if err := engine.Run(); err != nil {
			t.Fatal(err)
		}
		if agent.State.WriteLeft != 0 || agent.State.ReadLeft != 0 ||
			len(agent.State.PendingWriteReq) != 0 || len(agent.State.PendingReadReq) != 0 {
			t.Fatal("memory traffic did not complete")
		}
		if enabled {
			for _, tracker := range []*countingTracker{writes, reads} {
				if tracker.started != 8 || tracker.finished != 8 || tracker.inProgress != 0 {
					t.Fatalf("unexpected tracker counts: %+v", tracker)
				}
			}
			t.Log("UI-free trackers observed eight writes and eight reads, with no requests left in flight")
		}
	}
}
