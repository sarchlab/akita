package switches

import (
	"fmt"
	"testing"

	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/stretchr/testify/require"
)

// Drive whole middleware cycles with real ports, controlling downstream draining
// explicitly so throughput and backpressure can be observed cycle by cycle.
func throughputSwitch(t *testing.T, inputs, inputWidth, outputWidth, latency int) (*Comp, []*twowaybuffered.Port) {
	t.Helper()
	s := sim.MakeBuilder().Build()
	t.Cleanup(func() { require.NoError(t, s.Terminate()) })
	links := make([]Link, inputs+1)
	for i := range links {
		links[i] = Link{Remote: messaging.RemotePort(fmt.Sprintf("Remote%d", i)),
			NumInputChannel: inputWidth, NumOutputChannel: outputWidth, Latency: latency}
	}
	wiring := &testWiring{links: links, table: fixedRoute(fmt.Sprintf("Switch.Links[%d]", inputs))}
	sw := modeling.MakeBuilder[*Comp](DefaultSpec()).WithSimulation(s).
		WithResources(Resources{Wiring: wiring}).Build("Switch")
	ports := make([]*twowaybuffered.Port, len(links))
	for i := range ports {
		p := twowaybuffered.MakeBuilder().WithSimulation(s).
			WithSpec(twowaybuffered.Spec{IncomingCapacity: inputWidth, OutgoingCapacity: outputWidth}).Build()
		sw.BindPort(fmt.Sprintf("Links[%d]", i), p)
		(&tracingTestConnection{}).BindPort(p)
		ports[i] = p
	}
	require.NoError(t, s.Initialize())
	return sw, ports
}

func TestSwitchSustainedThroughput(t *testing.T) {
	for _, widths := range [][2]int{{1, 1}, {4, 4}, {4, 2}, {2, 4}} {
		for _, latency := range []int{0, 3} {
			t.Run(fmt.Sprintf("in%d_out%d_latency%d", widths[0], widths[1], latency), func(t *testing.T) {
				sw, ports := throughputSwitch(t, 1, widths[0], widths[1], latency)
				sent, received := uint64(0), uint64(0)
				steady := 0
				seen := make(map[uint64]bool)
				for cycle := 0; cycle < 200; cycle++ {
					if cycle < 60 {
						for ports[0].CanDeliver() {
							sent++
							ports[0].Deliver(messaging.Msg{ID: sent, Payload: packetization.Flit{Dst: "Destination"}})
						}
					}
					sw.Middlewares.RouteForwardSend.Handle(nil)
					sw.Middlewares.ReceivePipeline.Handle(nil)
					count := 0
					for {
						msg, ok := ports[1].RetrieveOutgoing()
						if !ok {
							break
						}
						received++
						require.False(t, seen[msg.ID], "duplicate flit")
						require.Greater(t, msg.ID, uint64(0))
						require.LessOrEqual(t, msg.ID, sent)
						seen[msg.ID] = true
						count++
					}
					require.LessOrEqual(t, count, widths[1])
					if cycle >= 20 && cycle < 60 {
						require.Equal(t, min(widths[0], widths[1]), count)
						steady += count
					}
				}
				require.Equal(t, sent, received)
				t.Logf("%d/%d widths, latency %d: %d flits in 40 steady cycles; all %d injected flits delivered",
					widths[0], widths[1], latency, steady, received)
			})
		}
	}
}

func TestSwitchContentionAndBackpressure(t *testing.T) {
	sw, ports := throughputSwitch(t, 2, 4, 2, 2)
	sent := [2]int{}
	received := [2]int{}
	blocked := false
	seen := make(map[uint64]bool)
	for cycle := 0; cycle < 300; cycle++ {
		for input := 0; input < 2; input++ {
			if cycle < 60 {
				for ports[input].CanDeliver() {
					sent[input]++
					ports[input].Deliver(messaging.Msg{ID: uint64(input*10000 + sent[input]),
						Payload: packetization.Flit{Dst: "Destination"}})
				}
			}
		}
		sw.Middlewares.RouteForwardSend.Handle(nil)
		sw.Middlewares.ReceivePipeline.Handle(nil)
		// Stop the consumer long enough to propagate backpressure to both inputs.
		if cycle >= 20 && cycle < 40 {
			if cycle == 39 {
				blocked = !ports[0].CanDeliver() && !ports[1].CanDeliver()
			}
			continue
		}
		count := 0
		for {
			msg, ok := ports[2].RetrieveOutgoing()
			if !ok {
				break
			}
			input := int(msg.ID) / 10000
			received[input]++
			require.False(t, seen[msg.ID], "duplicate flit")
			require.Greater(t, int(msg.ID)%10000, 0)
			require.LessOrEqual(t, int(msg.ID)%10000, sent[input])
			seen[msg.ID] = true
			count++
		}
		require.LessOrEqual(t, count, 2, "all contenders share the output budget")
		if cycle == 19 {
			require.Positive(t, received[0])
			require.Positive(t, received[1])
		}
	}
	require.True(t, blocked)
	require.Equal(t, sent, received)
	t.Logf("downstream stall reached both inputs; resumed and delivered %v flits without loss or duplicates", received)
}
