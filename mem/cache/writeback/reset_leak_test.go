package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/simulation/messaging"
	"github.com/sarchlab/akita/v5/simulation/modeling"
	"github.com/sarchlab/akita/v5/simulation/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/simulation/timing"
	"github.com/sarchlab/akita/v5/simulation/tracing"
	"github.com/sarchlab/akita/v5/simulation/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read MISS into the writeback cache
// so it opens a transaction and sends a fetch ReadReq out the Bottom port —
// leaving the req_in, the fetch req_out, and the directory-pipeline subtask
// open — then issues a Reset without answering the fetch and asserts every
// task the transaction opened is ended. A mid-flight Reset that drops the
// transaction table must leave no started-never-ended task (and no leaked
// receiver-registry entry).
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	storage := mem.NewStorage(1 * mem.MB)

	spec := Definition.DefaultSpec
	spec.TotalByteSize = 64 * 1024
	spec.NumBanks = 1
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.WayAssociativity = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1

	comp := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(Resources{
			Storage: storage,
			AddressToPortMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("LowerCache"),
			},
		}).
		WithPorts(makePorts("L1Cache", 16)).
		Build("L1Cache")
	plugNoopConn(comp)

	topPort := comp.Ports.Top
	ctrlPort := comp.Ports.Control

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Deliver a read that MISSES: the cache opens a transaction and forwards a
	// fetch ReadReq out the Bottom port. We never answer it, so req_in, the
	// fetch req_out, and the directory-pipeline subtask stay open.
	read := messaging.Msg{Payload: memprotocol.ReadReq{
		Address:        0x10000,
		AccessByteSize: 4},
		ID:  sim.NewID(),
		Src: messaging.RemotePort("Agent"),
		Dst: topPort.AsRemote(),

		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	topPort.Deliver(read)

	// Tick until the fetch is in flight: a transaction with HasFetchReadReq.
	// Do NOT answer the Bottom read; the request stays genuinely in flight.
	inFlight := false
	for i := 0; i < 64 && !inFlight; i++ {
		modelingtest.Tick(comp)
		for j := range comp.State.Transactions {
			if comp.State.Transactions[j].HasFetchReadReq {
				inFlight = true
				break
			}
		}
	}

	if !inFlight {
		t.Fatalf("expected an in-flight fetch transaction, none appeared")
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and fetch req_out to be opened, "+
			"got %d task starts", rec.NumStarted())
	}

	// Reset while the fetch is in flight.
	reset := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Cmd"),
		Dst:          ctrlPort.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	ctrlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		modelingtest.Tick(comp)
		if msg, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := msg.Payload.(memcontrolprotocol.Rsp); ok && rsp.
				Command == memcontrolprotocol.CmdReset {
				acked = true
			}
		}
	}

	if !acked {
		t.Fatal("Reset was not acked")
	}
	if open := rec.OpenTasks(); len(open) != 0 {
		t.Errorf("Reset left %d tracing task(s) unended: %s",
			len(open), rec.OpenSummary())
	}
}
