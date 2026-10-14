package endpoint

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	akitasim "github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/internal/tracingtest"
	"github.com/sarchlab/akita/v5/mem/memprotocol"
	"github.com/sarchlab/akita/v5/noc/packetization"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
)

func payloadMessage() messaging.Msg {
	return messaging.Msg{ID: 17, Src: "Sender.Port", Dst: "Receiver.Port", RspTo: 9,
		TrafficClass: "write", TrafficBytes: 96,
		Payload: memprotocol.WriteReq{Address: 128, Data: []byte("unique application payload")}}
}

func payloadFlits(msg messaging.Msg) []messaging.Msg {
	var id uint64 = 100
	spec := Definition.DefaultSpec
	spec.FlitByteSize = 32
	spec.EncodingOverhead = 0
	return msgToFlits(func() uint64 { id++; return id }, msg, spec, "Sender.NetworkPort", "Switch.Port", 42)
}

func TestOnlyHeadFlitCheckpointsOriginalMessage(t *testing.T) {
	msg := payloadMessage()
	flits := payloadFlits(msg)
	if len(flits) != 3 {
		t.Fatalf("got %d flits, want 3", len(flits))
	}
	for i, outer := range flits {
		part := outer.Payload.(packetization.Flit)
		if part.MsgID != msg.ID || part.Dst != msg.Dst || part.SeqID != i || part.NumFlitInMsg != 3 || part.MsgTaskID != 42 {
			t.Fatalf("flit %d has incorrect transport header: %+v", i, part)
		}
		if part.FlitTaskID != 0 {
			t.Fatal("packetization must not allocate tracing IDs")
		}
		if i == 0 {
			if !reflect.DeepEqual(part.Msg, msg) {
				t.Fatal("head lost original message")
			}
		} else if !reflect.DeepEqual(part.Msg, messaging.Msg{}) {
			t.Fatal("body carries a message")
		}
	}
	raw, err := json.Marshal(flits)
	if err != nil {
		t.Fatal(err)
	}
	// Every nested message is encoded under `msg`; the body has no such field.
	if got := bytes.Count(raw, []byte(`"msg":`)); got != 1 {
		t.Fatalf("serialized original message %d times: %s", got, raw)
	}
	var restored []messaging.Msg
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(restored, flits) {
		t.Fatal("flit checkpoint changed payload or header")
	}
	t.Logf("%d flits checkpointed with exactly one original message; body flits contain no msg field", len(flits))
}

type payloadOwner struct{ name string }

func (o payloadOwner) Name() string                { return o.name }
func (payloadOwner) NotifyRecv(messaging.Port)     {}
func (payloadOwner) NotifyPortFree(messaging.Port) {}

func newPayloadReceiver() (*Comp, messaging.Port, messaging.Port) {
	s := akitasim.MakeBuilder().WithTaskTracking().Build()
	device := twowaybuffered.NewPort(1, 1)
	device.BindOwner(payloadOwner{"Receiver"}, "Receiver.Port")
	network := twowaybuffered.NewPort(8, 8)
	ep := Definition.Builder().WithSimulation(s).
		WithResources(Resources{DevicePorts: []messaging.Port{device}}).
		Build("Endpoint")

	ep.BindPort("NetworkPort", network)
	ConnectDevices(ep)
	if err := s.Initialize(); err != nil {
		panic(err)
	}

	return ep, network, device
}

func TestReassemblyCheckpointAcrossArrivalOrders(t *testing.T) {
	for _, tc := range []struct {
		name    string
		order   []int
		payload any
	}{
		{"head-first", []int{0, 2, 1}, payloadMessage().Payload},
		{"head-middle", []int{2, 0, 1}, payloadMessage().Payload},
		{"head-last", []int{2, 1, 0}, payloadMessage().Payload},
		{"nil-payload", []int{2, 1, 0}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			msg := payloadMessage()
			msg.Payload = tc.payload
			flits := payloadFlits(msg)
			ep, network, device := newPayloadReceiver()
			// Force delivery backpressure after reassembly as well as checkpoint it.
			blocker := messaging.Msg{ID: 99}
			device.Deliver(blocker)
			for i, seq := range tc.order {
				network.Deliver(flits[seq])
				if !ep.Middlewares.Incoming.recv() {
					t.Fatal("did not consume flit")
				}
				if got := ep.Middlewares.Incoming.assemble(); got != (i == 2) {
					t.Fatalf("premature or missing completion after flit %d", seq)
				}
				if ep.Middlewares.Incoming.tryDeliver() {
					t.Fatal("delivered into full device port")
				}
				// Restore into fresh instances after each arrival, including both sides
				// of head arrival and the fully assembled-but-blocked state.
				ep, network, device = restorePayloadReceiver(t, ep, blocker)
			}
			device.RetrieveIncoming()
			if !ep.Middlewares.Incoming.tryDeliver() {
				t.Fatal("did not deliver completed message")
			}
			got, ok := device.RetrieveIncoming()
			if !ok || !reflect.DeepEqual(got, msg) {
				t.Fatalf("restored delivery = %#v, want %#v", got, msg)
			}
			if ep.Middlewares.Incoming.assemble() || ep.Middlewares.Incoming.tryDeliver() {
				t.Fatal("message delivered twice")
			}
			t.Log("original metadata and concrete payload survived head/body reordering, " +
				"repeated fresh-instance restore, and blocked delivery")
		})
	}
}

func TestDuplicateFlitCannotCompleteMessage(t *testing.T) {
	ep, network, _ := newPayloadReceiver()
	body := payloadFlits(payloadMessage())[1]
	network.Deliver(body)
	ep.Middlewares.Incoming.recv()
	network.Deliver(body)
	defer func() {
		if recover() == nil {
			t.Fatal("duplicate flit was counted as a new arrival")
		}
	}()
	ep.Middlewares.Incoming.recv()
}

func restorePayloadReceiver(t *testing.T, ep *Comp, blocker messaging.Msg) (*Comp, messaging.Port, messaging.Port) {
	t.Helper()
	var component bytes.Buffer
	if err := ep.SaveCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	next, network, device := newPayloadReceiver()
	device.Deliver(blocker)
	if err := next.LoadCheckpoint(&component); err != nil {
		t.Fatal(err)
	}
	return next, network, device
}

func TestTransportTaskIDsSurviveCheckpoint(t *testing.T) {
	ep, _, _ := newPayloadReceiver()
	rec := &tracingtest.LeakRecorder{}
	ep.TaskTracker().Subscribe(rec, ep)
	ep.State.MsgOutBuf = []messaging.Msg{payloadMessage()}
	if !ep.Middlewares.Outgoing.prepareFlits() {
		t.Fatal("did not packetize")
	}
	if rec.NumStarted() != 1+len(ep.State.FlitsToSend) {
		t.Fatal("message and flit tasks were not started")
	}
	restored, network, _ := restorePayloadReceiver(t, ep, messaging.Msg{ID: 99})
	restored.TaskTracker().Subscribe(&tracingtest.LeakRecorder{}, restored)
	for _, flit := range restored.State.FlitsToSend {
		part := flit.Payload.(packetization.Flit)
		if part.FlitTaskID == 0 || part.FlitTaskID == flit.ID {
			t.Fatal("flit task identity missing or equal to its message ID")
		}
		network.Deliver(flit)
		restored.Middlewares.Incoming.recv()
	}
	if !restored.Middlewares.Incoming.assemble() {
		t.Fatal("did not reassemble")
	}
	if len(rec.OpenTasks()) != 1+len(ep.State.FlitsToSend) {
		t.Fatal("restore must not fabricate old transport completions")
	}
	t.Log("restored flits retained handles and reassembled without requiring task history")
}
