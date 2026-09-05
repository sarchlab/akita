package monitoring2

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"

	"github.com/sarchlab/akita/v5/timing"
)

type serialTickComponent struct {
	*tickableComponent
	engine           *timing.SerialEngine
	entered, release chan struct{}
	active           bool
	events           int
}

func (c *serialTickComponent) Handle(evt timing.Event) {
	c.active = true
	if evt.Time() == 1 {
		close(c.entered)
		<-c.release
	}
	c.events++
	c.active = false
}

func (c *serialTickComponent) TickLater() {
	if c.active {
		panic("monitor scheduled while a model callback was active")
	}
	c.engine.Schedule(timing.MakeEventBase(c.engine.IDGenerator(), c.engine.CurrentTime()+1, c.Name()))
}

func TestMonitorTickWaitsForSerialCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := timing.NewSerialEngine()
		c := &serialTickComponent{
			tickableComponent: newTickableComponent("ticker"), engine: e,
			entered: make(chan struct{}), release: make(chan struct{}),
		}
		e.RegisterHandler(c.Name(), c)
		e.Schedule(timing.MakeEventBase(e.IDGenerator(), 1, c.Name()))
		m := NewMonitor()
		m.RegisterEngine(e)
		m.RegisterComponent(c)
		done := make(chan error, 1)
		go func() { done <- e.Run() }()
		<-c.entered
		w := httptest.NewRecorder()
		requested := make(chan struct{})
		go func() {
			m.containRequests(http.HandlerFunc(m.tick)).ServeHTTP(w,
				httptest.NewRequest(http.MethodPost, "/api/tick/ticker", nil))
			close(requested)
		}()
		synctest.Wait()
		select {
		case <-requested:
			t.Error("tick request completed before the active callback finished")
		default:
		}
		close(c.release)
		<-requested
		err := <-done
		if w.Code != http.StatusOK || err != nil || c.events != 2 {
			t.Fatalf("status=%d run=%v events=%d", w.Code, err, c.events)
		}
		t.Logf("HTTP tick waited for the active callback, returned 200, and its scheduled event completed")
	})
}
