package mmu

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

// TestResetEndsInflightTracingTasks drives a translation request into the MMU
// so its page walk is in flight and the req_in tracing task is open, then issues
// a Reset and asserts the task is ended — i.e. a mid-flight Reset that drops
// WalkingTranslations leaves no started-never-ended task. The MMU walk is a
// leaf page walk (no downstream req_out), so a single req_in is open per walk.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	reg := modeling.NewStandaloneRegistrar(engine)
	pageTable := vm.NewPageTable(12)

	// A long walk latency keeps the walk genuinely in flight after a single
	// parse tick, well short of completion.
	spec := DefaultSpec()
	spec.Latency = 100

	comp := MakeBuilder().
		WithRegistrar(reg).
		WithResources(Resources{PageTable: pageTable}).
		WithSpec(spec).
		Build("MMU")

	topPort := assignPort(reg, comp, "Top", 16)
	ctrlPort := assignPort(reg, comp, "Control", 4)
	(&noopConn{}).PlugIn(topPort)
	(&noopConn{}).PlugIn(ctrlPort)

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(comp, rec)

	// Insert a mapped page so the walk resolves locally once its countdown
	// elapses (no panic, no migration path), then admit the translation request.
	page := vm.Page{
		PID:      1,
		VAddr:    0x1000,
		PAddr:    0x1000,
		PageSize: 4096,
		Valid:    true,
		DeviceID: 0,
	}
	pageTable.Insert(page)

	req := vmprotocol.TranslationReq{}
	req.ID = timing.GetIDGenerator().Generate()
	req.Src = messaging.RemotePort("Agent")
	req.Dst = topPort.AsRemote()
	req.PID = 1
	req.VAddr = 0x1000
	req.DeviceID = 0
	req.TrafficClass = "vmprotocol.TranslationReq"
	topPort.Deliver(req)

	// One tick parses the request into a walk (opening the req_in) with a full
	// CycleLeft countdown of spec.Latency, so the walk is in flight but nowhere
	// near finished.
	comp.Tick()

	if len(comp.State.WalkingTranslations) != 1 {
		t.Fatalf("expected 1 in-flight walk, got %d",
			len(comp.State.WalkingTranslations))
	}
	if rec.NumStarted() < 1 {
		t.Fatalf("expected req_in to be opened, got %d task starts",
			rec.NumStarted())
	}

	// Reset while the walk is in flight.
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for range 64 {
		comp.Tick()
		if msg := ctrlPort.RetrieveOutgoing(); msg != nil {
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
