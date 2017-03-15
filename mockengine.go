package core

import (
	"log"
	"reflect"
)

// MockEngine is created to simplify the unit tests of other packages
type MockEngine struct {
	expectedEvent []Event
}

// NewMockEngine returns a new mock engine
func NewMockEngine() *MockEngine {
	e := new(MockEngine)
	e.expectedEvent = make([]Event, 0)
	return e
}

// ExpectSchedule register an event that is expected to be scheduled later.
func (e *MockEngine) ExpectSchedule(evt Event) {
	e.expectedEvent = append(e.expectedEvent, evt)
}

// Schedule of a MockEngine checks if the scheduling is expected or not
func (e *MockEngine) Schedule(evt Event) {
	for i, expected := range e.expectedEvent {
		if reflect.DeepEqual(expected, evt) {
			e.expectedEvent = append(e.expectedEvent[:i],
				e.expectedEvent[i+1:]...)
			return
		}
	}
	log.Panicf("Event %+v is not expected to be scheduled", evt)
}

// AllExpectedScheduled returns true if all the expected events are actually
// scheduled.
func (e *MockEngine) AllExpectedScheduled() bool {
	return len(e.expectedEvent) == 0
}

// Run function of a MockEngine does not do anything
func (e *MockEngine) Run() {
}
