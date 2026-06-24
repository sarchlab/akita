package mmuCache

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks delivers a Top translation request that
// misses the (empty) cache, so the mmuCache forwards it down the Bottom port
// and records it in State.InflightReqs — leaving the req_in and the forwarded
// req_out tracing tasks open. The Bottom response is never delivered, so the
// walk is genuinely in flight at Reset. It then issues a Reset and asserts both
// tasks are ended — i.e. a mid-flight Reset leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)

	spec := DefaultSpec()
	spec.NumBlocks = 1
	spec.NumLevels = 5
	spec.PageSize = 4096
	spec.Log2PageSize = 12
	spec.NumReqPerCycle = 4
	spec.LatencyPerLevel = 100

	comp := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(Resources{
			LowModulePort: messaging.RemotePort("LowModule"),
			UpModulePort:  messaging.RemotePort("UpModule"),
		}).
		Build("MMUCache")

	assignDefaultPorts(reg, comp)

	topPort := comp.GetPortByName("Top")
	bottomPort := comp.GetPortByName("Bottom")
	controlPort := comp.GetPortByName("Control")
	for _, p := range []messaging.Port{topPort, bottomPort, controlPort} {
		(&noopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Deliver a lookup that misses the empty cache. lookup forwards it out the
	// Bottom port and opens both the top req_in and the forwarded req_out, then
	// waits on the bottom response — which never comes, so the walk is in flight.
	req := vmprotocol.TranslationReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("Requester")
	req.Dst = topPort.AsRemote()
	req.PID = 1
	req.VAddr = 0x1000
	req.DeviceID = 1
	req.TrafficClass = "vmprotocol.TranslationReq"
	topPort.Deliver(req)

	// Tick until the forward happens and the walk is recorded in flight.
	forwarded := false
	for i := 0; i < 64 && !forwarded; i++ {
		comp.Tick()
		if len(comp.State.InflightReqs) > 0 {
			forwarded = true
		}
	}

	if !forwarded {
		t.Fatal("translation request was never forwarded down the Bottom port")
	}
	if len(comp.State.OutstandingBottomReqs) != 1 {
		t.Fatalf("expected 1 outstanding bottom request, got %d",
			len(comp.State.OutstandingBottomReqs))
	}
	if len(comp.State.InflightReqs) != 1 {
		t.Fatalf("expected 1 in-flight req, got %d",
			len(comp.State.InflightReqs))
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and req_out to be opened, got %d task starts",
			rec.NumStarted())
	}

	// Reset while the walk is in flight (bottom response deliberately withheld).
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = controlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	controlPort.Deliver(reset)

	acked := false
	for i := 0; i < 64 && !acked; i++ {
		comp.Tick()
		if msg := controlPort.RetrieveOutgoing(); msg != nil {
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
