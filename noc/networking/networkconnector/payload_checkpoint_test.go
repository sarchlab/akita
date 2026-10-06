package networkconnector_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/sarchlab/akita/v5/mem/memprotocol"
	nc "github.com/sarchlab/akita/v5/noc/networking/networkconnector"
	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type payloadOwner struct{ name string }

func (o payloadOwner) Name() string                { return o.name }
func (payloadOwner) NotifyRecv(messaging.Port)     {}
func (payloadOwner) NotifyPortFree(messaging.Port) {}

func payloadNetwork(t *testing.T) (*sim.Simulation, messaging.Port, messaging.Port) {
	t.Helper()
	s := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "trace.sqlite3")).Build()
	t.Cleanup(s.Terminate)
	src := twowaybuffered.NewPort("Sender.Port", 1, 4)
	dst := twowaybuffered.NewPort("Receiver.Port", 1, 4)
	for _, item := range []struct {
		name string
		port messaging.Port
	}{{"Sender", src}, {"Receiver", dst}} {
		item.port.SetOwner(payloadOwner{item.name})
		s.RegisterPort(item.port)
	}
	c := nc.MakeConnector().WithSimulation(s).WithFlitSize(16)
	c.NewNetwork("Network")
	left, right := c.AddSwitch(), c.AddSwitch()
	sw := nc.LinkEndSwitchParameter{IncomingBufSize: 1, OutgoingBufSize: 1,
		NumInputChannel: 1, NumOutputChannel: 1, Latency: 2}
	link := nc.LinkParameter{IsIdeal: true, Frequency: timing.GHz}
	dp := nc.DeviceToSwitchLinkParameter{
		DeviceEndParam: nc.LinkEndDeviceParameter{IncomingBufSize: 1, OutgoingBufSize: 1,
			NumInputChannel: 1, NumOutputChannel: 1},
		SwitchEndParam: sw, LinkParam: link,
	}
	c.ConnectDevice(left, []messaging.Port{src}, dp)
	c.ConnectDevice(right, []messaging.Port{dst}, dp)
	c.ConnectSwitches(left, right, nc.SwitchToSwitchLinkParameter{LeftEndParam: sw, RightEndParam: sw, LinkParam: link})
	c.EstablishRoute()
	return s, src, dst
}

func finishPayloadNetwork(t *testing.T, s *sim.Simulation, dst messaging.Port) []messaging.Msg {
	t.Helper()
	var got []messaging.Msg
	for attempt := 0; attempt < 5; attempt++ {
		if err := s.Engine().Run(); err != nil {
			t.Fatal(err)
		}
		msg, ok := dst.RetrieveIncoming()
		if !ok {
			return got
		}
		got = append(got, msg)
	}
	t.Fatal("network delivered extra messages")
	return nil
}

func checkpointPayloadCopies(t *testing.T, path string, payload []byte) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	z, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	defer z.Close()
	archive := tar.NewReader(z)
	count := 0
	for {
		_, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		count += bytes.Count(raw, []byte(base64.StdEncoding.EncodeToString(payload)))
	}
	return count
}

func TestNetworkPayloadDeliveryAndCheckpoint(t *testing.T) {
	// Snapshot before injection, with flits spread across the network, and
	// after the head can be held by the receiver while body flits are pending.
	for _, pause := range []timing.VTimeInPicoSec{0, 3000, 9000, 18000, 30000} {
		t.Run(fmt.Sprint(pause), func(t *testing.T) {
			s, src, dst := payloadNetwork(t)
			data := []byte("network checkpoint preserves this unique write payload")
			want := []messaging.Msg{
				{ID: s.NewID(), Src: src.AsRemote(), Dst: dst.AsRemote(), RspTo: 41,
					TrafficClass: "write", TrafficBytes: 160, Payload: memprotocol.WriteReq{Address: 128, Data: data}},
				{ID: s.NewID(), Src: src.AsRemote(), Dst: dst.AsRemote(),
					TrafficClass: "read", TrafficBytes: 16, Payload: memprotocol.ReadReq{Address: 256, AccessByteSize: 8}},
				{ID: s.NewID(), Src: src.AsRemote(), Dst: dst.AsRemote(), TrafficBytes: 0},
			}
			for _, msg := range want {
				src.Send(msg)
			}
			if pause > 0 {
				if err := s.Engine().(*timing.SerialEngine).RunUntil(pause); err != nil {
					t.Fatal(err)
				}
			}
			path := filepath.Join(t.TempDir(), "network.tar.gz")
			if err := s.SaveCheckpoint(path, "network-payload-test"); err != nil {
				t.Fatal(err)
			}
			if copies := checkpointPayloadCopies(t, path, data); copies != 1 {
				t.Fatalf("checkpoint contains payload %d times, want exactly one", copies)
			}
			uninterrupted := finishPayloadNetwork(t, s, dst)
			restored, _, restoredDst := payloadNetwork(t)
			if err := restored.LoadCheckpoint(path, "network-payload-test"); err != nil {
				t.Fatal(err)
			}
			resumed := finishPayloadNetwork(t, restored, restoredDst)
			if !reflect.DeepEqual(uninterrupted, want) || !reflect.DeepEqual(resumed, want) {
				t.Fatalf("full-message delivery mismatch: uninterrupted=%#v resumed=%#v want=%#v", uninterrupted, resumed, want)
			}
			if s.Engine().CurrentTime() != restored.Engine().CurrentTime() {
				t.Fatal("checkpoint changed completion time")
			}
			t.Logf("checkpoint at %d ps: exactly one stored write payload; "+
				"original write, read, and nil payloads delivered once across two switches; "+
				"resumed completion matches %d ps", pause, restored.Engine().CurrentTime())
		})
	}
}
