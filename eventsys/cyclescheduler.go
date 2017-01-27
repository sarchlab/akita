package eventsys

type CycleScheduler struct {
	Engine *Engine
	period VTimeInSec
}

func NewCycleScheduler(engine *Engine) *CycleScheduler {
	s := new(CycleScheduler)
	s.Engine = engine
	return s
}

func (s *CycleScheduler) Frequency() float64 {
	return 1.0 / float64(s.period)
}

func (s *CycleScheduler) SetFrequecy(freqInHZ float64) {
	s.period = VTimeInSec(1.0 / freqInHZ)
}

// Schedule registers the event to the event-driven simulation engine. The
// event will happen in number of cycles from current time
func (s *CycleScheduler) Schedule(event Event, numCycleFromNow uint64) {
	s.Engine.Schedule(event, VTimeInSec(numCycleFromNow)*s.period)
}

// Retry will make the event happen again in the next cycle
func (s *CycleScheduler) Retry(event Event) {
	s.Engine.Schedule(event, s.period)
}
