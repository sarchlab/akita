package event

// EventQueue is a priority queue of events. The front of the queue is always
// the event to happen next
type EventQueue []Event

// Len returns the length of the event queue
func (eq EventQueue) Len() int {
	return len(eq)
}

// Less determines the order between two events. Less returns true if the i-th
// event happens before the j-th event.
func (eq EventQueue) Less(i, j int) bool {
	return eq[i].Time() < eq[j].Time()
}

// Swap changes the position of two events in the event queue
func (eq EventQueue) Swap(i, j int) {
	eq[i], eq[j] = eq[j], eq[i]
}

// Push adds an event into the event queue
func (eq *EventQueue) Push(x interface{}) {
	event := x.(Event)
	*eq = append(*eq, event)
}

// Pop removes and returns the next event to happen
func (eq *EventQueue) Pop() interface{} {
	old := *eq
	n := len(old)
	event := old[n-1]
	*eq = old[0 : n-1]
	return event
}
