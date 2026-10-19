package sim_test

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/tasks"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// This owner deliberately has no tracker, clock, or ID-allocation methods.
type namedPortOwner struct{ name string }

func (o *namedPortOwner) Name() string { return o.name }
func (o *namedPortOwner) BindPort(slot string, p messaging.Port) {
	p.BindOwner(o, o.name+"."+slot)
}
func (*namedPortOwner) NotifyRecv(messaging.Port)     {}
func (*namedPortOwner) NotifyPortFree(messaging.Port) {}

type taskPortConnection struct{}

func (*taskPortConnection) Name() string                   { return "Link" }
func (c *taskPortConnection) BindPort(p messaging.Port)    { p.BindConnection(c) }
func (*taskPortConnection) NotifySend(messaging.Port)      {}
func (*taskPortConnection) NotifyAvailable(messaging.Port) {}

type portTaskEvents struct {
	tasks.NopObserver
	starts, ends []tasks.Task
}

func (r *portTaskEvents) TaskStarted(task tasks.Task) { r.starts = append(r.starts, task) }
func (r *portTaskEvents) TaskEnded(task tasks.Task)   { r.ends = append(r.ends, task) }

func TestRegisteredPortTracksWithoutOwnerTrackerMethod(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			builder := sim.MakeBuilder()
			if enabled {
				builder = builder.WithTaskTracking()
			}
			s := builder.Build()
			owner := &namedPortOwner{name: "Agent"}
			s.RegisterComponent(owner)
			p := twowaybuffered.NewPort(2, 2)
			owner.BindPort("Port", p)
			s.RegisterPort(p)
			(&taskPortConnection{}).BindPort(p)
			componentEvents, portEvents := &portTaskEvents{}, &portTaskEvents{}
			if enabled {
				s.TaskTracker().Subscribe(componentEvents, owner)
				s.TaskTracker().Subscribe(portEvents, p)
			}
			require.NoError(t, s.Initialize())
			advanceTaskClock(t, s, 10)
			outID, inID := s.NewID(), s.NewID()
			p.Send(messaging.Msg{ID: outID, Src: p.AsRemote(), Dst: "Peer.Port"})
			p.Deliver(messaging.Msg{ID: inID, Src: "Peer.Port", Dst: p.AsRemote()})
			nextID := s.NewID()
			if enabled {
				require.Equal(t, inID+3, nextID, "exactly two buffer tasks use the simulation allocator")
			} else {
				require.Equal(t, inID+1, nextID, "disabled port tracking allocates no IDs")
			}
			advanceTaskClock(t, s, 20)
			_, ok := p.RetrieveOutgoing()
			require.True(t, ok)
			_, ok = p.RetrieveIncoming()
			require.True(t, ok)
			if enabled {
				require.Len(t, portEvents.starts, 2)
				require.Len(t, portEvents.ends, 2)
				require.Equal(t, portEvents.starts, componentEvents.starts)
				require.Equal(t, portEvents.ends, componentEvents.ends)
				for i, direction := range []string{"outgoing", "incoming"} {
					ended := portEvents.ends[i]
					require.Equal(t, "Agent.Port."+direction, ended.Location)
					require.Equal(t, timing.VTimeInPicoSec(10), ended.StartTime)
					require.Equal(t, timing.VTimeInPicoSec(20), ended.EndTime)
					require.Equal(t, []uint64{outID, inID}[i], ended.MsgID)
				}
				t.Log("A custom owner without a tracker getter produced both port buffer tasks over [10,20], " +
					"visible to port and component subscribers")
			}
			require.NoError(t, s.Terminate())
		})
	}
}

func TestPortTrackerCannotBeReboundAcrossSimulations(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			builder := sim.MakeBuilder()
			if enabled {
				builder = builder.WithTaskTracking()
			}
			first, second := builder.Build(), sim.MakeBuilder().WithTaskTracking().Build()
			p := twowaybuffered.NewPort(1, 1)
			(&namedPortOwner{name: "Agent"}).BindPort("Port", p)
			first.RegisterPort(p)
			require.PanicsWithValue(t, "twowaybuffered: task tracker already bound", func() { second.RegisterPort(p) })
			require.Empty(t, second.Ports())
			require.NotPanics(t, func() { second.RequireNameAvailable(p.Name()) })
			require.NoError(t, first.Terminate())
			require.NoError(t, second.Terminate())
		})
	}
}

func TestRejectedPortNameDoesNotBindTracker(t *testing.T) {
	first, second := sim.MakeBuilder().WithTaskTracking().Build(), sim.MakeBuilder().Build()
	owner := &namedPortOwner{name: "Agent"}
	p, duplicate := twowaybuffered.NewPort(1, 1), twowaybuffered.NewPort(1, 1)
	owner.BindPort("Port", p)
	owner.BindPort("Port", duplicate)
	first.RegisterPort(p)
	require.Panics(t, func() { first.RegisterPort(duplicate) })
	require.NotPanics(t, func() { second.RegisterPort(duplicate) })
	require.NoError(t, first.Terminate())
	require.NoError(t, second.Terminate())
}
