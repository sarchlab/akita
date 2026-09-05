package timing

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/hooking"
)

type failureHandler func(Event)

func (f failureHandler) Handle(evt Event) { f(evt) }

type failureHook func(hooking.HookCtx)

func (f *failureHook) Func(ctx hooking.HookCtx) { (*f)(ctx) }

func engines() map[string]func() ManagedEngine {
	return map[string]func() ManagedEngine{
		"serial":   func() ManagedEngine { return NewSerialEngine() },
		"parallel": func() ManagedEngine { return NewParallelEngine() },
	}
}
func registrar(e ManagedEngine) HandlerRegistrar { return e.(HandlerRegistrar) }
func runWithDeadline(t *testing.T, fn func() error) error {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- fn() }()
	select {
	case err := <-done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("operation stranded workers or a paused dispatcher")
		return nil
	}
}
func expectPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("failed instance accepted operation")
		}
	}()
	fn()
}

func TestFailureIsIsolatedAcrossEngines(t *testing.T) {
	for name, newEngine := range engines() {
		t.Run(name, func(t *testing.T) {
			for _, where := range []string{"before hook", "handler", "after hook"} {
				t.Run(where, func(t *testing.T) {
					for _, cause := range []any{"bad state", errors.New("bad state"), struct{ Code int }{7}} {
						checkIsolatedFailure(t, newEngine, where, cause)
					}
				})
			}
		})
	}
}
func checkIsolatedFailure(t *testing.T, newEngine func() ManagedEngine, where string, cause any) {
	t.Helper()
	bad, good := newEngine(), newEngine()
	bad.Supervisor().SetID("bad")
	good.Supervisor().SetID("good")
	var after atomic.Int32
	hook := failureHook(func(ctx hooking.HookCtx) {
		if where == "before hook" && ctx.Pos == HookPosBeforeEvent {
			panic(cause)
		}
		if ctx.Pos == HookPosAfterEvent {
			after.Add(1)
			if where == "after hook" {
				panic(cause)
			}
		}
	})
	bad.AcceptHook(&hook)
	registrar(bad).RegisterHandler("component", failureHandler(func(Event) {
		if where == "handler" {
			panic(cause)
		}
	}))
	var goodCount atomic.Int32
	registrar(good).RegisterHandler("component", failureHandler(func(Event) { goodCount.Add(1) }))
	for i := 1; i <= 100; i++ {
		bad.Schedule(MakeEventBase(bad.IDGenerator(), VTimeInPicoSec(i), "component"))
		good.Schedule(MakeEventBase(good.IDGenerator(), VTimeInPicoSec(i), "component"))
	}
	goodDone := make(chan error, 1)
	go func() { goodDone <- good.Run() }()
	err := runWithDeadline(t, bad.Run)
	var failure *FailureError
	if !errors.As(err, &failure) || !reflect.DeepEqual(failure.Cause, cause) ||
		failure.SimulationID != "bad" || failure.Handler != "component" || failure.Time != 1 || len(failure.Stack) == 0 {
		t.Fatalf("missing failure context: %+v", err)
	}
	if where != "after hook" && after.Load() != 0 {
		t.Fatal("success hook ran after failure")
	}
	if err := runWithDeadline(t, func() error { return <-goodDone }); err != nil {
		t.Fatal(err)
	}
	if goodCount.Load() != 100 || good.CurrentTime() != 100 {
		t.Fatalf("unaffected simulation differs from control: %d events @ %d", goodCount.Load(), good.CurrentTime())
	}
	if bad.Run() == nil {
		t.Fatal("failed engine restarted")
	}
	expectPanic(t, func() { bad.Schedule(MakeEventBase(bad.IDGenerator(), 200, "component")) })
	expectPanic(t, bad.Continue)
	t.Logf("%s panic (%T): failed at event 1; independent simulation completed 100 events at time 100", where, cause)
}

func TestManagedWorkersCancelAndJoin(t *testing.T) {
	for name, newEngine := range engines() {
		t.Run(name, func(t *testing.T) {
			e := newEngine()
			var joined atomic.Bool
			started := make(chan struct{})
			registrar(e).RegisterHandler("component", failureHandler(func(Event) {
				if err := e.Supervisor().Go("waiting extension", func(ctx context.Context) error {
					close(started)
					<-ctx.Done()
					joined.Store(true)
					return nil
				}); err != nil {
					panic(err)
				}
				<-started
				panic("handler fails with worker active")
			}))
			e.Schedule(MakeEventBase(e.IDGenerator(), 1, "component"))
			if err := runWithDeadline(t, e.Run); err == nil || !joined.Load() {
				t.Fatalf("worker not joined on failure: %v, joined=%v", err, joined.Load())
			}
			if err := e.Supervisor().Go("late", func(context.Context) error { return nil }); err == nil {
				t.Fatal("late worker admitted")
			}
		})
	}
}

func TestWorkerCanWakeEmptyEngine(t *testing.T) {
	for name, newEngine := range engines() {
		t.Run(name, func(t *testing.T) {
			e := newEngine()
			var count atomic.Int32
			registrar(e).RegisterHandler("component", failureHandler(func(evt Event) {
				count.Add(1)
				if evt.Time() == 1 {
					if err := e.Supervisor().Go("producer", func(context.Context) error {
						time.Sleep(time.Millisecond)
						e.Schedule(MakeEventBase(e.IDGenerator(), 2, "component"))
						return nil
					}); err != nil {
						panic(err)
					}
				}
			}))
			e.Schedule(MakeEventBase(e.IDGenerator(), 1, "component"))
			if err := runWithDeadline(t, e.Run); err != nil {
				t.Fatal(err)
			}
			if count.Load() != 2 {
				t.Fatalf("run returned before worker event: %d", count.Load())
			}
		})
	}
}

func TestCancelWakesPausedEngine(t *testing.T) {
	for name, newEngine := range engines() {
		t.Run(name, func(t *testing.T) {
			e := newEngine()
			e.Schedule(MakeEventBase(e.IDGenerator(), 1, "unused"))
			e.Pause()
			done := make(chan error, 1)
			go func() { done <- e.Run() }()
			e.Supervisor().Cancel()
			err := runWithDeadline(t, func() error { return <-done })
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("cancel lost: %v", err)
			}
		})
	}
}

func TestParallelSaturatedSchedulingAndHandlerSerialization(t *testing.T) {
	e := NewParallelEngine()
	var active atomic.Int32
	var count atomic.Int32
	e.RegisterHandler("same", failureHandler(func(evt Event) {
		if active.Add(1) != 1 {
			panic("same handler ran concurrently")
		}
		defer active.Add(-1)
		count.Add(1)
		if evt.Time() == 1 {
			e.Schedule(MakeEventBase(e.IDGenerator(), 2, "same"))
		}
	}))
	for i := 0; i < 20000; i++ {
		e.Schedule(MakeEventBase(e.IDGenerator(), 1, "same"))
	}
	if err := runWithDeadline(t, e.Run); err != nil {
		t.Fatal(err)
	}
	if count.Load() != 40000 {
		t.Fatalf("lost scheduled work: %d", count.Load())
	}
}

func TestConcurrentWorkerFailuresPreserveFirstCause(t *testing.T) {
	s := NewSupervisor("workers")
	sentinel := errors.New("primary")
	err := runWithDeadline(t, func() error {
		return s.Execute("setup", func() error {
			var ready sync.WaitGroup
			ready.Add(8)
			start := make(chan struct{})
			for i := 0; i < 8; i++ {
				if err := s.Go(fmt.Sprintf("worker %d", i), func(context.Context) error {
					ready.Done()
					<-start
					panic(i)
				}); err != nil {
					return err
				}
			}
			ready.Wait()
			s.Fail("first", sentinel)
			close(start)
			return nil
		})
	})
	if !errors.Is(err, sentinel) || len(s.Failures()) != 9 {
		t.Fatalf("failure history lost: %v, count=%d", err, len(s.Failures()))
	}
	if err := s.Close(func() { panic("cleanup") }); !errors.Is(err, sentinel) || len(s.Failures()) != 10 {
		t.Fatal("cleanup replaced first cause")
	}
}

func TestCloseCancelsAndJoinsActiveOperation(t *testing.T) {
	s := NewSupervisor("active")
	entered := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- s.Execute("run", func() error { close(entered); <-s.Context().Done(); return nil }) }()
	<-entered
	if err := runWithDeadline(t, func() error { return s.Close(func() {}) }); !errors.Is(err, context.Canceled) {
		t.Fatalf("active termination reported success: %v", err)
	}
	if err := runWithDeadline(t, func() error { return <-done }); !errors.Is(err, context.Canceled) {
		t.Fatalf("running operation lost cancellation: %v", err)
	}
}

func TestPauseWaitsForAdmittedHandler(t *testing.T) {
	for name, newEngine := range engines() {
		t.Run(name, func(t *testing.T) {
			e := newEngine()
			entered := make(chan struct{})
			release := make(chan struct{})
			registrar(e).RegisterHandler("blocked", failureHandler(func(Event) { close(entered); <-release }))
			e.Schedule(MakeEventBase(e.IDGenerator(), 1, "blocked"))
			done := make(chan error, 1)
			go func() { done <- e.Run() }()
			<-entered
			paused := make(chan struct{})
			go func() { e.Pause(); close(paused) }()
			select {
			case <-paused:
				t.Error("pause returned while handler still touched model state")
			case <-time.After(time.Millisecond):
			}
			close(release)
			if err := runWithDeadline(t, func() error { <-paused; e.Continue(); return nil }); err != nil {
				t.Fatal(err)
			}
			if err := runWithDeadline(t, func() error { return <-done }); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFinishedProducerWakeIsNotLost(t *testing.T) {
	s := NewSupervisor("test")
	wake := make(chan struct{}, 1)
	// The queue was empty, then the last owned producer scheduled and exited.
	wake <- struct{}{}
	if !s.waitForWork(wake) {
		t.Fatal("finished producer's final event would remain unprocessed")
	}
	if s.waitForWork(wake) {
		t.Fatal("no remaining work")
	}
}
