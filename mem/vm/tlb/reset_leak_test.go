package tlb

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a translation lookup into the TLB so
// it misses and an MSHR entry with an outstanding bottom fetch is in flight —
// the request's req_in (and a pipeline subtask, if still staged) plus the
// shadow req_out for the bottom fetch are all open. It then issues a Reset and
// asserts every open task is ended, i.e. a mid-flight Reset leaves no
// started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	remotePort := messaging.RemotePort("MMU")

	tlbComp := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(DefaultSpec()).
		WithResources(Resources{
			TranslationProviderMapper: &mem.SinglePortMapper{
				Port: remotePort,
			},
		}).
		Build("TLB")

	assignDefaultPorts(reg, tlbComp)
	plugNoopConn(tlbComp)

	topPort := tlbComp.GetPortByName("Top")
	bottomPort := tlbComp.GetPortByName("Bottom")
	controlPort := tlbComp.GetPortByName("Control")

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(tlbComp, rec)

	// Deliver a lookup that misses (fresh TLB), so it is staged into the lookup
	// pipeline (opening req_in + a pipeline subtask) and, once it reaches the
	// lookup, creates an MSHR entry and forwards a bottom fetch (opening the
	// shadow req_out). The bottom fetch is never answered, so the miss stays in
	// flight.
	req := vmprotocol.TranslationReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("Agent")
	req.Dst = topPort.AsRemote()
	req.PID = 1
	req.VAddr = 0x1000
	req.DeviceID = 1
	req.TrafficClass = "vmprotocol.TranslationReq"
	topPort.Deliver(req)

	// Tick just far enough for the miss to reach an MSHR entry with a bottom
	// fetch sent; do not answer the fetch.
	bottomSent := false
	for i := 0; i < 64 && mshrIsEmpty(tlbComp.State.MSHREntries); i++ {
		tlbComp.Tick()
		if out := bottomPort.RetrieveOutgoing(); out != nil {
			if _, ok := out.(vmprotocol.TranslationReq); ok {
				bottomSent = true
			}
		}
	}

	// Confirm the request is genuinely in flight: an MSHR entry exists.
	if mshrIsEmpty(tlbComp.State.MSHREntries) {
		t.Fatal("expected an in-flight MSHR entry before Reset, got none")
	}
	if !bottomSent {
		t.Fatal("expected a bottom fetch to have been forwarded before Reset")
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected at least req_in and req_out to be opened, "+
			"got %d task starts", rec.NumStarted())
	}

	// Reset while the miss is in flight.
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = controlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	controlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64; i++ {
		tlbComp.Tick()
		if msg := controlPort.RetrieveOutgoing(); msg != nil {
			if rsp, ok := msg.(memcontrolprotocol.Rsp); ok &&
				rsp.Command == memcontrolprotocol.CmdReset {
				acked = true
				break
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
