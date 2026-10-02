package writethroughcache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read miss into the writethrough
// cache so the transaction has both of its tracing tasks open — the req_in for
// the top request and the downstream req_out for the bottom fetch — then issues
// a Reset and asserts every task is ended. A mid-flight Reset drops the
// transaction table, so each started task must be ended by endInflightTasks or
// it leaks (and, for a req_in, leaks a receiver-registry entry too).
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)
	storage := mem.NewStorage(4 * mem.GB)

	spec := Definition.DefaultSpec
	spec.NumReqPerCycle = 1
	spec.NumBanks = 1
	spec.NumMSHREntry = 8
	spec.WayAssociativity = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1
	spec.TotalByteSize = 64 * 1024
	spec.MaxNumConcurrentTrans = 16

	comp := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(Resources{
			Storage: storage,
			AddressMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("LowerCache"),
			},
		}).
		WithPorts(makePorts("L1Cache", 16)).
		Build("L1Cache")

	// Plug each port into a no-op connection before the component is ticked.
	topPort := comp.Ports.Top
	bottomPort := comp.Ports.Bottom
	ctrlPort := comp.Ports.Control
	for _, p := range []messaging.Port{topPort, bottomPort, ctrlPort} {
		(&ccNoopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Admit a read miss. intake opens the req_in; the directory then issues a
	// bottom fetch (req_out) that is never answered, leaving the transaction in
	// flight with both its req_in and req_out tracing tasks open.
	read := memprotocol.ReadReq{Address: 0, AccessByteSize: 4}
	read.ID = sim.NewID()
	read.Src = messaging.RemotePort("Agent")
	read.Dst = topPort.AsRemote()
	read.TrafficBytes = 12
	read.TrafficClass = "memprotocol.ReadReq"
	topPort.Deliver(read)

	// Tick until the bottom fetch has been sent, so the in-flight transaction
	// has HasReadToBottom set and its req_out task is open. Do NOT answer the
	// bottom request.
	bottomSent := false
	for i := 0; i < 256 && !bottomSent; i++ {
		modelingtest.Tick(comp)
		if out, ok := bottomPort.RetrieveOutgoing(); ok {
			if _, ok := out.(memprotocol.ReadReq); ok {
				bottomSent = true
			}
		}
	}

	if !bottomSent {
		t.Fatal("bottom fetch was never issued; transaction not in flight")
	}

	// The transaction is genuinely in flight: a slot exists that is not yet
	// removed and has issued its downstream read.
	inflight := false
	for i := range comp.State.Transactions {
		trans := &comp.State.Transactions[i]
		if !trans.Removed && trans.HasReadToBottom {
			inflight = true
		}
	}
	if !inflight {
		t.Fatal("expected an in-flight transaction with a bottom read")
	}

	// Before the Reset the transaction's req_in and its downstream req_out are
	// open (the dir_pipeline subtask already closed when the fetch was issued).
	if open := rec.OpenTasks(); len(open) < 2 {
		t.Fatalf("expected req_in and req_out to be open, got %d: %s",
			len(open), rec.OpenSummary())
	}

	// Reset while the transaction is in flight.
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = sim.NewID()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		modelingtest.Tick(comp)
		if msg, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := msg.(memcontrolprotocol.Rsp); ok &&
				rsp.Command == memcontrolprotocol.CmdReset {
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
