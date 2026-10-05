package gmmu

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/sarchlab/akita/v5/sim/tracing"
	"github.com/sarchlab/akita/v5/sim/tracing/tracingtest"
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
	sim := modeling.NewStandaloneSimulation(engine)
	pageTable := vm.NewPageTable(12)

	spec := Definition.DefaultSpec
	spec.DeviceID = deviceID
	spec.Latency = 1
	spec.LowModule = lowModule

	comp := Definition.Builder().
		WithSimulation(sim).
		WithResources(Resources{PageTable: pageTable}).
		WithSpec(spec).
		WithPorts(defaultPorts("GMMU")).
		Build("GMMU")

	for _, p := range allPorts(comp) {
		(&noopConn{}).PlugIn(p)
	}

	topPort := comp.Ports.Top
	ctrlPort := comp.Ports.Control

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

	req := messaging.Msg{Payload: vmprotocol.TranslationReq{
		PID:      1,
		VAddr:    vAddr,
		DeviceID: deviceID},
		ID:  sim.NewID(),
		Src: agentPort,
		Dst: topPort.AsRemote(),

		TrafficClass: "vmprotocol.TranslationReq"}

	topPort.Deliver(req)

	// Tick until the remote memory request has been issued: the walk has left
	// WalkingTranslations and now sits in RemoteMemReqs, with both the original
	// req_in and the downstream req_out tracing tasks open.
	for i := 0; i < 64 && len(comp.State.RemoteMemReqs) == 0; i++ {
		modelingtest.Tick(comp)
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
