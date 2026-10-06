// Command reqtracing shows the four-stage request lifecycle.
//
// A Client sends requests to a Server, one at a time, over a direct
// connection; the Server replies after a fixed latency. Both sides annotate
// the request with the tracing.TraceReq* helpers:
//
//	Client send : TraceReqInitiate  -> opens a "req_out" task (round trip)
//	Server recv : TraceReqReceive   -> opens a "req_in"  task (handling)
//	Server done : TraceReqComplete  -> ends the "req_in"  task
//	Client recv : TraceReqFinalize  -> ends the "req_out" task
//
// Two AverageTimeTracers — one filtering "req_out" on the client, one
// filtering "req_in" on the server — then report the round-trip latency and
// the server handling time from the same run.
//
// The two components live in the client and server packages, one component
// per package; this file is the system builder that wires them together.
package main

import (
	"fmt"

	"github.com/sarchlab/akita/v5/examples/reqtracing/client"
	"github.com/sarchlab/akita/v5/examples/reqtracing/server"
	"github.com/sarchlab/akita/v5/sim/messaging/direct"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
)

func main() {
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	// Build the server first, so the client's Spec can name its port.
	srv := server.Definition.Builder().
		WithSimulation(sim).
		WithPorts(server.Ports{Out: twowaybuffered.NewPort("Server.Out", 4, 4)}).
		Build("Server")

	clientSpec := client.Definition.DefaultSpec
	clientSpec.Dst = srv.Ports.Out.AsRemote()
	clientSpec.NumReqs = 3

	cli := client.Definition.Builder().
		WithSimulation(sim).
		WithSpec(clientSpec).
		WithPorts(client.Ports{Out: twowaybuffered.NewPort("Client.Out", 4, 4)}).
		Build("Client")

	conn := direct.Definition.Builder().WithSimulation(sim).Build("Conn")
	conn.PlugIn(cli.Ports.Out)
	conn.PlugIn(srv.Ports.Out)

	// The filter is how each tracer selects the tasks it cares about.
	roundTrip := tracing.NewAverageTimeTracer(
		func(t tracing.TaskStart) bool { return t.Kind == "req_out" })
	handling := tracing.NewAverageTimeTracer(
		func(t tracing.TaskStart) bool { return t.Kind == "req_in" })
	tracing.CollectTrace(cli, roundTrip)
	tracing.CollectTrace(srv, handling)

	// The client sends on its own, so start it; the server wakes when a
	// request arrives.
	cli.TickLater()

	if err := engine.Run(); err != nil {
		panic(err)
	}

	fmt.Printf("requests completed:           %d\n", roundTrip.TotalCount())
	fmt.Printf("avg round trip (req_out):     %d ps\n", roundTrip.AverageTime())
	fmt.Printf("avg server handling (req_in): %d ps\n", handling.AverageTime())
}
