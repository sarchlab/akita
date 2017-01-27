package eventsys

import "container/heap"

type eventQueue []Event

func (eq eventQueue) Len() int {
	return len(eq)
}

func (eq eventQueue) Less(i, j int) bool {
	return eq[i].Time() < eq[j].Time()
}

func (eq eventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
}

func (eq *eventQueue) Push(x interface{}) {
	event := x.(Event)
	*eq = append(*eq, event)
}

func (eq *eventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	event := old[n-1]
	*eq = old[0 : n-1]
	return event
}

// An Engine is the unit that maintains all the events and runs all the events
// in the simulation
type Engine struct {
	now   VTimeInSec
	queue eventQueue
}

// NewEngine creates a new event-driven simulator engine
func NewEngine() *Engine {
	e := new(Engine)
	e.queue = make(eventQueue, 0, 1000)
	heap.Init(&e.queue)
	return e
}

// Now returns the current time in the simuated world
func (engine *Engine) Now() VTimeInSec {
	return engine.now
}

// Schedule registers an event. The event will happen in a certain number
// of seconds from now
func (engine *Engine) Schedule(event Event, secFromNow VTimeInSec) {
	event.SetTime(secFromNow + engine.now)
	heap.Push(&engine.queue, event)
}

// HasMoreEvent checkes if there are more event scheduled in the Engine
func (engine *Engine) HasMoreEvent() bool {
	return len(engine.queue) > 0
}

// Run will let the next event happen
func (engine *Engine) Run() {
	if len(engine.queue) > 0 {
		event := heap.Pop(&engine.queue).(Event)
		engine.now = event.Time()
		event.Happen()
	}
}

// Reset will force remove all the events in the engine and set the engine
// time to 0
func (engine *Engine) Reset() {
	engine.queue = make(eventQueue, 0, 1000)
	heap.Init(&engine.queue)
	engine.now = 0
}
