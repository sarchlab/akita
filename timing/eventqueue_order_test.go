package timing

import (
	"reflect"
	"testing"
)

// orderEvent carries an id so a test can observe the order events are popped or
// handled in.
type orderEvent struct {
	t  VTimeInSec
	id int
}

func (e *orderEvent) Time() VTimeInSec  { return e.t }
func (e *orderEvent) HandlerID() string { return "h" }
func (e *orderEvent) IsSecondary() bool { return false }

func TestEventQueueOrdersByTimeThenSchedule(t *testing.T) {
	q := newUnsafeEventQueue()

	// Several events share a time; they must come out in schedule (push) order.
	pushes := []*orderEvent{
		{t: 5, id: 0},
		{t: 2, id: 1},
		{t: 5, id: 2},
		{t: 2, id: 3},
		{t: 5, id: 4},
		{t: 1, id: 5},
	}
	for _, e := range pushes {
		q.Push(e)
	}

	var gotIDs []int
	for q.Len() > 0 {
		gotIDs = append(gotIDs, q.Pop().(*orderEvent).id)
	}

	// time order, ties broken by schedule order.
	wantIDs := []int{5, 1, 3, 0, 2, 4}
	if !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("pop order = %v, want %v", gotIDs, wantIDs)
	}
}

type orderRecordingHandler struct {
	order []int
}

func (h *orderRecordingHandler) Handle(e Event) error {
	h.order = append(h.order, e.(*orderEvent).id)
	return nil
}

func TestSerialEngineFiresSameTimeInScheduleOrder(t *testing.T) {
	engine := NewSerialEngine()
	handler := &orderRecordingHandler{}
	engine.RegisterHandler("h", handler)

	for id := 0; id < 8; id++ {
		engine.Schedule(&orderEvent{t: 10, id: id})
	}

	if err := engine.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	want := []int{0, 1, 2, 3, 4, 5, 6, 7}
	if !reflect.DeepEqual(handler.order, want) {
		t.Fatalf("handle order = %v, want %v", handler.order, want)
	}
}
