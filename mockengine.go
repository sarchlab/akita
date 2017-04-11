package core

// MockEngine is created to simplify the unit tests of other packages
type MockEngine struct {
	ScheduledEvent []Event
}

// NewMockEngine returns a new mock engine
func NewMockEngine() *MockEngine {
	e := new(MockEngine)
	e.ScheduledEvent = make([]Event, 0)
	return e
}

// Schedule of a put a the scheduled event in the ScheduledEvent list
func (e *MockEngine) Schedule(evt Event) {
	e.ScheduledEvent = append(e.ScheduledEvent, evt)
}

// Run function of a MockEngine does not do anything
func (e *MockEngine) Run() error {
	return nil
}

// Pause function of MockEngine is not implemented
func (e *MockEngine) Pause() {

}
