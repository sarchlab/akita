package twowaybuffered

import (
	"testing"

	"github.com/sarchlab/akita/v5/sim/hooking"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

type benchmarkConnection struct{ hooking.HookableBase }

func (*benchmarkConnection) Name() string                   { return "benchmark" }
func (*benchmarkConnection) BindPort(messaging.Port)        {}
func (*benchmarkConnection) NotifyAvailable(messaging.Port) {}
func (*benchmarkConnection) NotifySend()                    {}

type benchmarkPayload struct {
	Data []byte
}

// BenchmarkSend measures Send plus resetting the outgoing buffer for reuse.
func BenchmarkSend(b *testing.B) {
	p := NewPort(1, 1)
	p.BindOwner(&portPresenceOwner{}, "Src")
	p.BindConnection(&benchmarkConnection{})
	payload := benchmarkPayload{Data: make([]byte, 64)}
	msg := messaging.Msg{ID: 1, Src: "Src", Dst: "dst", TrafficBytes: 64, Payload: payload}
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
	msg := messaging.Msg{ID: 1, Src: "Src", Dst: "dst", TrafficBytes: 64, Payload: payload}
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		p := NewPort(1, 1)
		p.BindOwner(&portPresenceOwner{}, "Src")
		p.BindConnection(&benchmarkConnection{})
		for pb.Next() {
			p.Send(msg)
			p.outgoingBuf.Pop()
		}
	})
}
