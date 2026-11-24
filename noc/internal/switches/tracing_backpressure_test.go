package switches

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/internal/testsim"
	"github.com/sarchlab/akita/v5/internal/tracingtest"
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
func (*tracingTestConnection) NotifySend(_ messaging.Port)    {}
func (*tracingTestConnection) NotifyAvailable(messaging.Port) {}
func (c *tracingTestConnection) BindPort(p messaging.Port)    { p.BindConnection(c) }

func TestTrackedSwitchBackpressure(t *testing.T) {
	for _, latency := range []int{0, 1} {
		t.Run(fmt.Sprintf("latency_%d", latency), func(t *testing.T) {
			clock := testsim.NewClock()
			s := testsim.New(clock, true)
			port := twowaybuffered.MakeBuilder().WithSimulation(s).Build()
			wiring := &testWiring{
				links: []Link{{
					Remote:           "Destination",
					Latency:          latency,
					NumInputChannel:  1,
					NumOutputChannel: 1}},
				table: fixedRoute("Switch.Links[0]")}
			sw := modeling.MakeBuilder[*Comp](Spec{Freq: timing.GHz}).WithSimulation(s).
				WithResources(Resources{Wiring: wiring}).Build("Switch")
			sw.BindPort("Links[0]", port)
			(&tracingTestConnection{}).BindPort(port)
			require.NoError(t, s.Initialize())
			rec := &tracingtest.LeakRecorder{}
			s.TaskTracker().Subscribe(rec, sw)

			flit := func() messaging.Msg {
				return messaging.Msg{ID: s.NewID(), Src: "Source", Dst: port.AsRemote(),
					Payload: packetization.Flit{Dst: "Destination"}}
			}
			admitted := 1
			if latency == 0 {
				admitted = sw.State.PortComplexes[0].Waiting.Capacity()
			}
			var want []uint64
			for range admitted {
				msg := flit()
				want = append(want, msg.ID)
				port.Deliver(msg)
				require.True(t, sw.Middlewares.ReceivePipeline.startProcessing())
			}
			last := flit()
			want = append(want, last.ID)
			port.Deliver(last)
			for range 10 {
				require.False(t, sw.Middlewares.ReceivePipeline.startProcessing())
			}
			require.Equal(t, 1, port.IncomingBuf().Size())
			require.Equal(t, admitted, rec.NumStarted(), "blocked attempts must not start phantom tasks")
			require.Len(t, rec.OpenTasks(), admitted)

			var delivered []uint64
			for range 20 {
				clock.Step(sw.Spec.Freq)
				sw.Middlewares.RouteForwardSend.Handle(nil)
				sw.Middlewares.ReceivePipeline.Handle(nil)
				if msg, ok := port.RetrieveOutgoing(); ok {
					delivered = append(delivered, msg.ID)
				}
			}
			require.Equal(t, want, delivered)
			require.Zero(t, port.IncomingBuf().Size())
			require.Equal(t, len(want), rec.NumStarted())
			require.Empty(t, rec.OpenTasks(), rec.OpenSummary())
		})
	}
}
