package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
	"github.com/sarchlab/akita/v5/modeling/modelingtest"
	"github.com/sarchlab/akita/v5/timing"
	"github.com/sarchlab/akita/v5/tracing"
	"github.com/sarchlab/akita/v5/tracing/tracingtest"
)

// TestResetEndsInflightTracingTasks drives a read into the address translator so
// its req_in and the translation req_out tasks are open (a transaction awaiting
// translation), then issues a Reset and asserts both tasks are ended — i.e. a
// mid-flight Reset leaves no started-never-ended task.
func TestResetEndsInflightTracingTasks(t *testing.T) { //nolint:funlen
	engine := timing.NewSerialEngine()
	sim := modeling.NewStandaloneSimulation(engine)

	spec := Definition.DefaultSpec
	spec.Log2PageSize = 12
	spec.Freq = 1

	resources := Resources{
		MemProviderMapper: &mem.SinglePortMapper{
			Port: messaging.RemotePort("MemPort"),
		},
		TranslationProviderMapper: &mem.SinglePortMapper{
			Port: messaging.RemotePort("TranslationProvider"),
		},
	}

	at := Definition.Builder().
		WithSimulation(sim).
		WithSpec(spec).
		WithResources(resources).
		WithPorts(makePorts("AddressTranslator", 16)).
		Build("AddressTranslator")

	topPort := at.Ports.Top
	translationPort := at.Ports.Translation
	ctrlPort := at.Ports.Control

	for _, p := range []messaging.Port{
		topPort,
		at.Ports.Bottom,
		translationPort,
		ctrlPort,
	} {
		(&noopConn{}).PlugIn(p)
	}

	rec := &tracingtest.LeakRecorder{}
	tracing.CollectTrace(at, rec)

	// Admit a read: translate opens the top req_in and the translation req_out,
	// then awaits the translation response (which never comes — the request is in
	// flight). Tick until the transaction is created and the TranslationReq has
	// actually gone out the Translation port.
	read := messaging.Msg{Payload: memprotocol.ReadReq{Address: 0x1040, AccessByteSize: 4},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Agent"),
		Dst:          topPort.AsRemote(),
		TrafficBytes: 12,
		TrafficClass: "memprotocol.ReadReq"}

	topPort.Deliver(read)

	var transReqSent bool
	for range 8 {
		modelingtest.Tick(at)
		if out, ok := translationPort.RetrieveOutgoing(); ok {
			if _, ok := out.Payload.(vmprotocol.TranslationReq); ok {
				transReqSent = true
				break
			}
		}
	}

	if !transReqSent {
		t.Fatal("translation request was not sent out the Translation port")
	}
	if len(at.State.Transactions) != 1 {
		t.Fatalf("expected 1 in-flight transaction, got %d",
			len(at.State.Transactions))
	}
	if rec.NumStarted() < 2 {
		t.Fatalf("expected req_in and translation req_out to be opened, "+
			"got %d task starts", rec.NumStarted())
	}

	// Reset while the transaction is in flight (translation never answered).
	reset := messaging.Msg{Payload: memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset},
		ID:           sim.NewID(),
		Src:          messaging.RemotePort("Cmd"),
		Dst:          ctrlPort.AsRemote(),
		TrafficClass: "memcontrolprotocol.Req"}

	ctrlPort.Deliver(reset)

	acked := false
	for range 64 {
		modelingtest.Tick(at)
		if msg, ok := ctrlPort.RetrieveOutgoing(); ok {
			if rsp, ok := msg.Payload.(memcontrolprotocol.Rsp); ok && rsp.
				Command == memcontrolprotocol.CmdReset {
				acked = true
				break
			}
		}
	}

	if !acked {
		t.Fatal("Reset was not acked")
	}
	if len(at.State.Transactions) != 0 {
		t.Errorf("Reset left %d transaction(s) in state",
			len(at.State.Transactions))
	}
	if open := rec.OpenTasks(); len(open) != 0 {
		t.Errorf("Reset left %d tracing task(s) unended: %s",
			len(open), rec.OpenSummary())
	}
}
