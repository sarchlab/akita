package writeback

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
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
	storage := mem.NewStorage(1 * mem.MB)

	spec := DefaultSpec()
	spec.TotalByteSize = 64 * 1024
	spec.NumBanks = 1
	spec.NumMSHREntry = 16
	spec.NumReqPerCycle = 4
	spec.WayAssociativity = 2
	spec.Log2BlockSize = 6
	spec.BankLatency = 1
	spec.DirLatency = 1

	comp := MakeBuilder().
		WithRegistrar(modeling.NewStandaloneRegistrar(engine)).
		WithSpec(spec).
		WithResources(Resources{
			Storage: storage,
			AddressToPortMapper: &mem.SinglePortMapper{
				Port: messaging.RemotePort("LowerCache"),
			},
		}).
		Build("L1Cache")

	for _, name := range []string{"Top", "Bottom", "Control"} {
		comp.AssignPort(name,
			messaging.NewPort(comp, 16, 16, comp.Name()+"."+name))
	}
	topPort := comp.GetPortByName("Top")
	botPort := comp.GetPortByName("Bottom")
	ctrlPort := comp.GetPortByName("Control")
	for _, p := range []messaging.Port{topPort, botPort, ctrlPort} {
		(&ccNoopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Deliver a read that MISSES: the cache opens a transaction and forwards a
	// fetch ReadReq out the Bottom port. We never answer it, so req_in, the
	// fetch req_out, and the directory-pipeline subtask stay open.
	read := memprotocol.ReadReq{}
	read.ID = timing.GetIDGenerator().Generate()
	read.Src = messaging.RemotePort("Agent")
	read.Dst = topPort.AsRemote()
	read.Address = 0x10000
	read.AccessByteSize = 4
	read.TrafficBytes = 12
	read.TrafficClass = "memprotocol.ReadReq"
	topPort.Deliver(read)

	// Tick until the fetch is in flight: a transaction with HasFetchReadReq.
	// Do NOT answer the Bottom read; the request stays genuinely in flight.
	inFlight := false
	for i := 0; i < 64 && !inFlight; i++ {
		comp.Tick()
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
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		comp.Tick()
		if msg := ctrlPort.RetrieveOutgoing(); msg != nil {
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
