package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a translation into the GMMU until it
// issues a remote memory request out the Bottom port — so the original req_in
// and the downstream req_out tracing tasks are both open — then issues a Reset
// without ever answering the remote request and asserts both tasks are ended.
// A mid-flight Reset must leave no started-never-ended task (and no leaked
// receiver-registry entry).
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	const (
		deviceID  = uint64(0)
		lowModule = messaging.RemotePort("LowModule")
		agentPort = messaging.RemotePort("Agent")
		vAddr     = uint64(0x10000000)
	)

	engine := timing.NewSerialEngine()
	pageTable := vm.NewPageTable(12)

	spec := DefaultSpec()
	spec.DeviceID = deviceID
	spec.Latency = 1
	spec.LowModule = lowModule

	reg := modeling.NewStandaloneRegistrar(engine)
	comp := MakeBuilder().
		WithRegistrar(reg).
		WithResources(Resources{PageTable: pageTable}).
		WithSpec(spec).
		Build("GMMU")

	assignDefaultPorts(reg, comp)
	for _, name := range []string{"Top", "Bottom", "Control"} {
		(&noopConn{}).PlugIn(comp.GetPortByName(name))
	}

	topPort := comp.GetPortByName("Top")
	ctrlPort := comp.GetPortByName("Control")

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// The page lives on a different device, so the walk cannot resolve locally:
	// it must send a remote memory request out the Bottom port and wait for a
	// response (which never comes — the request is left in flight).
	pageTable.Insert(vm.Page{
		PID:      vm.PID(1),
		VAddr:    vAddr,
		DeviceID: 1,
		Valid:    true,
	})

	req := vmprotocol.TranslationReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = agentPort
	req.Dst = topPort.AsRemote()
	req.PID = 1
	req.VAddr = vAddr
	req.DeviceID = deviceID
	req.TrafficClass = "vmprotocol.TranslationReq"
	topPort.Deliver(req)

	// Tick until the remote memory request has been issued: the walk has left
	// WalkingTranslations and now sits in RemoteMemReqs, with both the original
	// req_in and the downstream req_out tracing tasks open.
	for i := 0; i < 64 && len(comp.State.RemoteMemReqs) == 0; i++ {
		comp.Tick()
	}

	if len(comp.State.RemoteMemReqs) == 0 {
		t.Fatalf("expected a remote memory request in flight, got none")
	}
	// req_in (TraceReqReceive in parseFromTop) and req_out (TraceReqInitiate in
	// processRemoteMemReq) should both be open.
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and req_out to be opened, got %d task starts",
			rec.NumStarted())
	}
	if open := rec.OpenTasks(); len(open) == 0 {
		t.Fatalf("expected open tasks before reset, got none")
	}

	// Reset while the remote walk is in flight; the remote response is never
	// delivered.
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
