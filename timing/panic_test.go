package timing

import (
	"bytes"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/hooking"
)

type panicHandler func(Event)

func (h panicHandler) Handle(e Event) { h(e) }

type panicHook func(hooking.HookCtx)

func (h panicHook) Func(ctx hooking.HookCtx) { h(ctx) }

type testEngine interface {
	Engine
	HandlerRegistrar
}

func finishRun(t *testing.T, run func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- run() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("engine did not finish after panic")
		return nil
	}
}

func catchPanic(t *testing.T, fn func()) (cause any) {
	t.Helper()
	defer func() {
		cause = recover()
		if cause == nil {
			t.Error("expected panic")
		}
	}()
	fn()
	return nil
}

func TestEnginePanicIsolation(t *testing.T) {
	factories := map[string]func() testEngine{
		"serial":   func() testEngine { return NewSerialEngine() },
		"parallel": func() testEngine { return NewParallelEngine() },
	}
	for name, build := range factories {
		t.Run(name, func(t *testing.T) {
			for _, phase := range []string{"before", "handler", "after"} {
				t.Run(phase, func(t *testing.T) {
					for _, cause := range []any{errors.New("model failure"), "model failure", struct{ Code int }{7}} {
						checkEnginePanic(t, build, phase, cause)
					}
				})
			}
		})
	}
}

func checkEnginePanic(t *testing.T, build func() testEngine, phase string, cause any) {
	t.Helper()
	bad, good := build(), build()
	var goodEvents, afterHooks atomic.Int32
	bad.RegisterHandler("model", panicHandler(func(Event) {
		if phase == "handler" {
			panic(cause)
		}
	}))
	bad.AcceptHook(panicHook(func(ctx hooking.HookCtx) {
		if phase == "before" && ctx.Pos == HookPosBeforeEvent {
			panic(cause)
		}
		if ctx.Pos == HookPosAfterEvent {
			afterHooks.Add(1)
			if phase == "after" {
				panic(cause)
			}
		}
	}))
	good.RegisterHandler("model", panicHandler(func(Event) { goodEvents.Add(1) }))
	for i := 1; i <= 100; i++ {
		bad.Schedule(EventBase{ID: uint64(i), Time_: VTimeInPicoSec(i), HandlerID_: "model"})
		good.Schedule(EventBase{ID: uint64(i), Time_: VTimeInPicoSec(i), HandlerID_: "model"})
	}
	goodDone := make(chan error, 1)
	go func() { goodDone <- good.Run() }()
	err := finishRun(t, bad.Run)
	var failure *PanicError
	if !errors.As(err, &failure) || !reflect.DeepEqual(failure.Cause, cause) || len(failure.Stack) == 0 ||
		failure.Handler != "model" || failure.Time != 1 || failure.EventType == "" {
		t.Fatalf("missing panic diagnostics: %+v", err)
	}
	if wrapped, ok := cause.(error); ok && !errors.Is(err, wrapped) {
		t.Fatal("lost original error identity")
	}
	if phase != "after" && afterHooks.Load() != 0 {
		t.Fatal("success hook ran after failure")
	}
	if err := finishRun(t, func() error { return <-goodDone }); err != nil {
		t.Fatal(err)
	}
	if goodEvents.Load() != 100 || good.CurrentTime() != 100 {
		t.Fatalf("peer completed %d events at %d", goodEvents.Load(), good.CurrentTime())
	}
	if bad.Run() != err {
		t.Fatal("failed engine accepted another run or lost its first failure")
	}
	catchPanic(t, func() { bad.Schedule(EventBase{Time_: 200, HandlerID_: "model"}) })
	t.Logf("%s panic (%T) contained at time 1; independent engine completed 100 events at time 100", phase, cause)
}

func TestRunUntilContainsPanicAndRejectsReuse(t *testing.T) {
	e := NewSerialEngine()
	e.RegisterHandler("model", panicHandler(func(Event) { panic("broken model") }))
	e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
	err := finishRun(t, func() error { return e.RunUntil(1) })
	if err == nil || e.RunUntil(2) != err {
		t.Fatal("RunUntil did not preserve terminal failure")
	}
	var out bytes.Buffer
	if e.SaveCheckpoint(&out) != err || out.Len() != 0 {
		t.Fatal("failed engine produced a checkpoint")
	}
	if e.LoadCheckpoint(bytes.NewBufferString("{}")) != err {
		t.Fatal("failed engine accepted restore")
	}
}

// This queue makes dispatcher failure deterministic while a worker is waiting
// for a queue token. Healthy scheduling still uses the original queue design.
type panicDispatchQueue struct{ events []Event }

func (q *panicDispatchQueue) Len() int     { return len(q.events) }
func (q *panicDispatchQueue) Peek() Event  { return q.events[0] }
func (q *panicDispatchQueue) Pop() Event   { e := q.events[0]; q.events = q.events[1:]; return e }
func (q *panicDispatchQueue) Push(e Event) { q.events = append(q.events, e) }

type panicTimeEvent struct {
	EventBase
	attempted <-chan struct{}
}

func (e panicTimeEvent) Time() VTimeInPicoSec { <-e.attempted; panic("dispatcher failure") }

func TestParallelDispatcherPanicReleasesWaitingWorker(t *testing.T) {
	e := NewParallelEngine()
	attempted := make(chan struct{})
	var joined atomic.Bool
	e.RegisterHandler("model", panicHandler(func(Event) {
		defer joined.Store(true)
		close(attempted)
		e.Schedule(EventBase{Time_: 2, HandlerID_: "model"})
	}))
	q := &panicDispatchQueue{events: []Event{
		EventBase{Time_: 1, HandlerID_: "model"},
		panicTimeEvent{attempted: attempted},
	}}
	e.queues = []EventQueue{q}
	e.queueChan = make(chan EventQueue, 1)
	e.queueChan <- q
	err := finishRun(t, e.Run)
	var failure *PanicError
	if !errors.As(err, &failure) || failure.Cause != "dispatcher failure" || !joined.Load() {
		t.Fatalf("dispatcher did not contain panic and join worker: %v joined=%v", err, joined.Load())
	}
	if !e.pauseLock.TryLock() {
		t.Fatal("round lock was left held")
	}
	e.pauseLock.Unlock()
	t.Log("dispatcher failure released and joined a worker blocked in Schedule")
}

func TestEventQueueReleasesExistingLockOnPanic(t *testing.T) {
	for _, op := range []string{"peek", "pop", "push"} {
		t.Run(op, func(t *testing.T) {
			q := NewEventQueue()
			catchPanic(t, func() {
				switch op {
				case "peek":
					q.Peek()
				case "pop":
					q.Pop()
				case "push":
					q.Push(EventBase{Time_: 1})
					ready := make(chan struct{})
					close(ready)
					q.Push(panicTimeEvent{attempted: ready})
				}
			})
			if !q.TryLock() {
				t.Fatal("queue lock was left held after panic")
			}
			q.Unlock()
		})
	}
}
