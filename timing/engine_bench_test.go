package timing

import "testing"

type dispatchBenchHandler struct {
	engine *SerialEngine
	left   int
}

func (h *dispatchBenchHandler) Handle(evt Event) {
	h.left--
	if h.left > 0 {
		next := evt.Time() + 1
		h.engine.Schedule(EventBase{ID: uint64(next), Time_: next, HandlerID_: "bench"})
	}
}
func BenchmarkSerialEngineEvents(b *testing.B) {
	e := NewSerialEngine()
	h := &dispatchBenchHandler{engine: e}
	e.RegisterHandler("bench", h)
	const perRun = 10000
	b.ReportAllocs()
	for b.Loop() {
		h.left = perRun
		next := e.CurrentTime() + 1
		e.Schedule(EventBase{ID: uint64(next), Time_: next, HandlerID_: "bench"})
		if err := e.Run(); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*perRun), "ns/event")
}
