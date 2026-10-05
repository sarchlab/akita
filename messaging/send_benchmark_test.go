package messaging

import (
	"github.com/sarchlab/akita/v5/hooking"
	"testing"
)

type benchmarkConnection struct{ hooking.HookableBase }

func (*benchmarkConnection) Name() string         { return "benchmark" }
func (*benchmarkConnection) PlugIn(Port)          {}
func (*benchmarkConnection) NotifyAvailable(Port) {}
func (*benchmarkConnection) NotifySend()          {}

type benchmarkPayload struct {
	Data []byte
}

func init() { msgCodec.Register(benchmarkPayload{}) }

// BenchmarkSend measures Send plus resetting the outgoing buffer for reuse.
func BenchmarkSend(b *testing.B) {
	p := NewPort("src", 1, 1).(*defaultPort)
	p.SetConnection(&benchmarkConnection{})
	payload := benchmarkPayload{Data: make([]byte, 64)}
	msg := Msg{ID: 1, Src: "src", Dst: "dst", TrafficBytes: 64, Payload: payload}
	b.ReportAllocs()
	for b.Loop() {
		p.Send(msg)
		p.outgoingBuf.Pop()
	}
}

// BenchmarkSendParallel gives each sender its own port so only registration
// lookup is shared across senders, as it is in a parallel simulation.
func BenchmarkSendParallel(b *testing.B) {
	payload := benchmarkPayload{Data: make([]byte, 64)}
	msg := Msg{ID: 1, Src: "src", Dst: "dst", TrafficBytes: 64, Payload: payload}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		p := NewPort("src", 1, 1).(*defaultPort)
		p.SetConnection(&benchmarkConnection{})
		for pb.Next() {
			p.Send(msg)
			p.outgoingBuf.Pop()
		}
	})
}
