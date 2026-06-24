package addresstranslator

import (
	"testing"

	"github.com/sarchlab/akita/v5/mem"
	"github.com/sarchlab/akita/v5/mem/memcontrolprotocol"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/mem/vm/vmprotocol"
	"github.com/sarchlab/akita/v5/messaging"
	"github.com/sarchlab/akita/v5/modeling"
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
	reg := modeling.NewStandaloneRegistrar(engine)

	spec := DefaultSpec()
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

	at := MakeBuilder().
		WithRegistrar(reg).
		WithSpec(spec).
		WithResources(resources).
		Build("AddressTranslator")

	assignPorts(reg, at, 16)

	topPort := at.GetPortByName("Top")
	translationPort := at.GetPortByName("Translation")
	ctrlPort := at.GetPortByName("Control")

	for _, p := range []messaging.Port{
		topPort,
		at.GetPortByName("Bottom"),
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
	read := memprotocol.ReadReq{Address: 0x1040, AccessByteSize: 4}
	read.ID = timing.GetIDGenerator().Generate()
	read.Src = messaging.RemotePort("Agent")
	read.Dst = topPort.AsRemote()
	read.TrafficBytes = 12
	read.TrafficClass = "memprotocol.ReadReq"
	topPort.Deliver(read)

	var transReqSent bool
	for range 8 {
		at.Tick()
		if out := translationPort.RetrieveOutgoing(); out != nil {
			if _, ok := out.(vmprotocol.TranslationReq); ok {
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
	reset := memcontrolprotocol.Req{Command: memcontrolprotocol.CmdReset}
	reset.ID = timing.GetIDGenerator().Generate()
	reset.Src = messaging.RemotePort("Cmd")
	reset.Dst = ctrlPort.AsRemote()
	reset.TrafficClass = "memcontrolprotocol.Req"
	ctrlPort.Deliver(reset)

	acked := false
	for range 64 {
		at.Tick()
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
	if len(at.State.Transactions) != 0 {
		t.Errorf("Reset left %d transaction(s) in state",
			len(at.State.Transactions))
	}
	if open := rec.OpenTasks(); len(open) != 0 {
		t.Errorf("Reset left %d tracing task(s) unended: %s",
			len(open), rec.OpenSummary())
	}
}
