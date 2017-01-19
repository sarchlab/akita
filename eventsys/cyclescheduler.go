package eventsys

type CycleScheduler struct {
	Engine *Engine
	period float64
}

func NewCycleScheduler(engine *Engine) *CycleScheduler {
	s := new(CycleScheduler)
	s.Engine = engine
	return s
}

func (s *CycleScheduler) Frequency() float64 {
	return 1.0 / s.period
}

func (s *CycleScheduler) SetFrequecy(freqInHZ float64) {
	s.period = 1.0 / freqInHZ
}

// Schedule registers the event to the event-driven simulation engine. The
// event will happen in number of cycles from current time
func (s *CycleScheduler) Schedule(event Event, numCycleFromNow uint64) {
	s.Engine.Schedule(event, float64(numCycleFromNow)*s.period)
}

// Retry will make the event happen again in the next cycle
func (s *CycleScheduler) Retry(event Event) {
	s.Engine.Schedule(event, s.period)
}
