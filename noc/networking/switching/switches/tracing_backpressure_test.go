package switches

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/noc/networking/routing"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

// The test drives middleware cycles and drains the real port explicitly.
type tracingTestConnection struct{ hooking.HookableBase }

func (*tracingTestConnection) Name() string                   { return "Link" }
func (*tracingTestConnection) NotifySend()                    {}
func (*tracingTestConnection) NotifyAvailable(messaging.Port) {}
func (c *tracingTestConnection) BindPort(p messaging.Port)    { p.BindConnection(c) }

func TestTrackedSwitchBackpressure(t *testing.T) {
	for _, latency := range []int{0, 1} {
		t.Run(fmt.Sprintf("latency_%d", latency), func(t *testing.T) {
			s := modeling.NewStandaloneSimulation(timing.NewSerialEngine()).WithTaskTracking()
			routes := routing.NewTable()
			sw := Definition.Builder().WithSimulation(s).WithResources(Resources{
				RoutingTable: routes,
				Links: []Link{{Remote: "Destination", Latency: latency,
					NumInputChannel: 1, NumOutputChannel: 1}},
			}).Build("Switch")
			port := twowaybuffered.NewPort(1, 1)
			sw.BindPort("Port[0]", port)
			(&tracingTestConnection{}).BindPort(port)
			routes.DefineDefaultRoute(port.AsRemote())
			require.NoError(t, s.Initialize())
			rec := &tracingtest.LeakRecorder{}
			s.TaskTracker().Subscribe(rec, sw)

			flit := func() messaging.Msg {
				return messaging.Msg{ID: s.NewID(), Src: "Source", Dst: port.AsRemote(),
					TrafficClass: "packetization.Flit", Payload: packetization.Flit{Dst: "Destination"}}
			}
			first, second := flit(), flit()
			port.Deliver(first)
			require.True(t, sw.Middlewares.ReceivePipeline.startProcessing())
			require.Equal(t, 1, rec.NumStarted())
			port.Deliver(second)

			// The first flit occupies either the zero-latency route buffer or
			// the pipeline input. Retrying the second must not create tasks.
			for range 10 {
				require.False(t, sw.Middlewares.ReceivePipeline.startProcessing())
			}
			require.Equal(t, 1, port.NumIncoming())
			require.Equal(t, 1, rec.NumStarted(), "blocked attempts must not start phantom tasks")
			require.Len(t, rec.OpenTasks(), 1)

			var delivered []uint64
			for range 20 {
				sw.Middlewares.RouteForwardSend.Handle(nil)
				sw.Middlewares.ReceivePipeline.Handle(nil)
				if msg, ok := port.RetrieveOutgoing(); ok {
					delivered = append(delivered, msg.ID)
				}
			}
			require.Equal(t, []uint64{first.ID, second.ID}, delivered)
			require.Zero(t, port.NumIncoming())
			require.Equal(t, 2, rec.NumStarted())
			require.Empty(t, rec.OpenTasks(), rec.OpenSummary())
			t.Logf("latency %d: 10 blocked retries, 2 admitted flits delivered, 2 tasks ended, no open tasks", latency)
		})
	}
}
