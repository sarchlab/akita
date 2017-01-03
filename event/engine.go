package event

import (
	"container/heap"
)

type EventQueue []Event

func (eq EventQueue) Len() int {
	return len(eq)
}

func (eq EventQueue) Less(i, j int) bool {
	return eq[i].Time() > eq[j].Time()
}

func (eq EventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
}

func (eq *EventQueue) Push(x interface{}) {
	event := x.(Event)
	*eq = append(*eq, event)
}

func (eq *EventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	event := old[n-1]
	*eq = old[0 : n-1]
	return event
}

type Engine struct {
	queue EventQueue
}

func NewEngine() *Engine {
	e := new(Engine)
	e.queue = make(EventQueue, 1000)
	heap.Init(&e.queue)
	return e
}

func (engine *Engine) RegisterEvent(event Event) {
	heap.Push(&engine.queue, event)
}

func (engine *Engine) Run() {
	for len(engine.queue) > 0 {
		event := heap.Pop(&engine.queue).(Event)
		event.Happen()
	}
}
