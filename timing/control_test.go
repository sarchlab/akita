package timing

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/hooking"
)

func controlEngines(t *testing.T, test func(*testing.T, testEngine)) {
	t.Helper()
	for name, build := range map[string]func() testEngine{
		"serial":   func() testEngine { return NewSerialEngine() },
		"parallel": func() testEngine { return NewParallelEngine() },
	} {
		t.Run(name, func(t *testing.T) { test(t, build()) })
	}
}

func controlContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func receiveControl[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		t.Fatal("control operation did not complete")
		var zero T
		return zero
	}
}

// Synchronize the test's injection with a queued inspection, not a sleep that
// may release a handler before the inspection goroutine has reached the engine.
func awaitInspection(t *testing.T, e testEngine) {
	t.Helper()
	var c *engineControl
	switch e := e.(type) {
	case *SerialEngine:
		c = e.engineControl
	case *ParallelEngine:
		c = e.engineControl
	}
	ctx := controlContext(t)
	for {
		c.mu.Lock()
		found := false
		for _, req := range c.queue {
			found = found || req.kind == controlInspect
		}
		c.mu.Unlock()
		if found {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("inspection was not queued")
		case <-time.After(time.Millisecond):
		}
	}
}

func startControlRun(e Engine) <-chan error {
	done := make(chan error, 1)
	go func() { done <- e.Run() }()
	return done
}

func TestPauseAcknowledgesAfterHooksAndServesInspection(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		ctx := controlContext(t)
		entered, release := make(chan struct{}), make(chan struct{})
		hookEntered, releaseHook := make(chan struct{}), make(chan struct{})
		var events int
		e.RegisterHandler("model", panicHandler(func(evt Event) {
			events++
			if evt.Time() == 1 {
				close(entered)
				<-release
			}
		}))
		e.AcceptHook(panicHook(func(ctx hooking.HookCtx) {
			if ctx.Pos == HookPosAfterEvent && ctx.Item.(Event).Time() == 1 {
				close(hookEntered)
				<-releaseHook
			}
		}))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		e.Schedule(EventBase{Time_: 2, HandlerID_: "model"})
		done := startControlRun(e)
		receiveControl(t, entered)
		pause := e.RequestPause()
		select {
		case <-pause.(*pauseRequest).done:
			t.Fatal("pause acknowledged inside handler")
		default:
		}
		if e.IsPaused() {
			t.Fatal("requested pause reported as acknowledged")
		}
		close(release)
		receiveControl(t, hookEntered)
		select {
		case <-pause.(*pauseRequest).done:
			t.Fatal("pause acknowledged inside after hook")
		default:
		}
		close(releaseHook)
		requireControlSuccess(t, pause.Wait(ctx))
		if err := pause.Wait(ctx); err != nil {
			t.Fatal("acknowledgment is not reusable", err)
		}
		if !e.IsPaused() {
			t.Fatal("pause was not recorded")
		}
		requireControlSuccess(t, e.Pause())
		assertPausedSnapshot(t, e, &events)
		requireControlSuccess(t, e.Continue())
		requireControlSuccess(t, receiveControl(t, done))
		if events != 2 {
			t.Fatal("continue did not finish remaining event")
		}
		requireControlSuccess(t, e.Continue())
		t.Log("pause acknowledged after handler and after-hook; " +
			"inspected time 1 while paused; continue completed event 2")
	})
}

func TestHandlerCanRequestPause(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		ticket := make(chan PauseRequest, 1)
		e.RegisterHandler("model", panicHandler(func(evt Event) {
			if evt.Time() == 1 {
				ticket <- e.RequestPause()
			}
		}))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		e.Schedule(EventBase{Time_: 2, HandlerID_: "model"})
		done := startControlRun(e)
		if err := receiveControl(t, ticket).Wait(controlContext(t)); err != nil {
			t.Fatal(err)
		}
		if !e.IsPaused() {
			t.Fatal("handler pause was lost")
		}
		if err := e.Continue(); err != nil {
			t.Fatal(err)
		}
		if err := receiveControl(t, done); err != nil {
			t.Fatal(err)
		}
	})
}

func TestInspectionWaitsForConsistentState(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan struct{})
		var left, right int
		e.RegisterHandler("model", panicHandler(func(Event) {
			left = 1
			close(entered)
			<-release
			right = 1
		}))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		done := startControlRun(e)
		receiveControl(t, entered)
		inspected := make(chan error, 1)
		ctx := controlContext(t)
		go func() {
			inspected <- e.Inspect(ctx, func() error {
				if left != 1 || right != 1 {
					return errors.New("partially updated state")
				}
				return nil
			})
		}()
		awaitInspection(t, e)
		close(release)
		if err := receiveControl(t, inspected); err != nil {
			t.Fatal(err)
		}
		if err := receiveControl(t, done); err != nil {
			t.Fatal(err)
		}
		if e.IsPaused() {
			t.Fatal("inspection changed pause state")
		}
		t.Log("inspection observed both completed field updates, even when the last event ended Run")
	})
}

func TestFailedRunSettlesPendingControls(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan struct{})
		cause := errors.New("model failure during pause request")
		e.RegisterHandler("model", panicHandler(func(Event) { close(entered); <-release; panic(cause) }))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		done := startControlRun(e)
		receiveControl(t, entered)
		pause := e.RequestPause()
		inspected := make(chan error, 1)
		ctx := controlContext(t)
		go func() { inspected <- e.Inspect(ctx, func() error { t.Error("inspected failed model"); return nil }) }()
		awaitInspection(t, e)
		close(release)
		err := receiveControl(t, done)
		if !errors.Is(err, cause) {
			t.Fatal(err)
		}
		if pause.Wait(ctx) != err || receiveControl(t, inspected) != err || e.Continue() != err {
			t.Fatal("pending control lost run failure")
		}
		if e.IsPaused() {
			t.Fatal("failed engine reported paused")
		}
	})
}

func TestCancelledInspectionAndPauseWait(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		entered, release := make(chan struct{}), make(chan struct{})
		e.RegisterHandler("model", panicHandler(func(Event) { close(entered); <-release }))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		done := startControlRun(e)
		receiveControl(t, entered)
		ctx, cancel := context.WithCancel(controlContext(t))
		inspection := make(chan error, 1)
		go func() { inspection <- e.Inspect(ctx, func() error { t.Error("cancelled callback ran"); return nil }) }()
		awaitInspection(t, e)
		pause := e.RequestPause()
		cancel()
		if !errors.Is(receiveControl(t, inspection), context.Canceled) {
			t.Fatal("inspection wait was not cancelled")
		}
		if !errors.Is(pause.Wait(ctx), context.Canceled) {
			t.Fatal("pause wait was not cancelled")
		}
		close(release)
		if err := receiveControl(t, done); err != nil {
			t.Fatal(err)
		}
		if err := pause.Wait(controlContext(t)); err != nil {
			t.Fatal(err)
		}
		if !e.IsPaused() {
			t.Fatal("cancelling a wait cancelled the pause request")
		}
		if err := e.Continue(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestIdleInspectionExcludesRunAndContainsCallbackPanic(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		ctx := controlContext(t)
		entered, release := make(chan struct{}), make(chan struct{})
		var inspecting atomic.Bool
		e.RegisterHandler("model", panicHandler(func(Event) {
			if inspecting.Load() {
				t.Error("Run overlapped idle inspection")
			}
		}))
		e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
		inspection := make(chan error, 1)
		go func() {
			inspection <- e.Inspect(ctx, func() error {
				inspecting.Store(true)
				close(entered)
				<-release
				inspecting.Store(false)
				return nil
			})
		}()
		receiveControl(t, entered)
		done := startControlRun(e)
		close(release)
		if err := receiveControl(t, inspection); err != nil {
			t.Fatal(err)
		}
		if err := receiveControl(t, done); err != nil {
			t.Fatal(err)
		}
		cause := errors.New("serializer failure")
		if err := e.Inspect(ctx, func() error { panic(cause) }); !errors.Is(err, cause) {
			t.Fatal("inspection lost panic", err)
		}
		if err := e.Run(); err != nil {
			t.Fatal("read-only inspection failure invalidated engine", err)
		}
	})
}

func TestRunUntilSettlesControlsAtLimit(t *testing.T) {
	e := NewSerialEngine()
	var ticket PauseRequest
	e.RegisterHandler("model", panicHandler(func(Event) { ticket = e.RequestPause() }))
	e.Schedule(EventBase{Time_: 1, HandlerID_: "model"})
	e.Schedule(EventBase{Time_: 2, HandlerID_: "model"})
	if err := e.RunUntil(1); err != nil {
		t.Fatal(err)
	}
	if err := ticket.Wait(controlContext(t)); err != nil {
		t.Fatal(err)
	}
	if !e.IsPaused() {
		t.Fatal("RunUntil lost pending pause at its time limit")
	}
	if err := e.Inspect(controlContext(t), func() error {
		if e.CurrentTime() != 1 {
			t.Error("RunUntil advanced past limit")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestParallelPauseWaitsForEveryWorker(t *testing.T) {
	e := NewParallelEngine()
	slowEntered, release := make(chan struct{}), make(chan struct{})
	ticket := make(chan PauseRequest, 1)
	e.RegisterHandler("slow", panicHandler(func(Event) { close(slowEntered); <-release }))
	e.RegisterHandler("pause", panicHandler(func(evt Event) {
		if evt.Time() == 1 {
			ticket <- e.RequestPause()
		}
	}))
	e.Schedule(EventBase{ID: 1, Time_: 1, HandlerID_: "slow"})
	e.Schedule(EventBase{ID: 2, Time_: 1, HandlerID_: "pause"})
	e.Schedule(EventBase{ID: 3, Time_: 2, HandlerID_: "pause"})
	done := startControlRun(e)
	receiveControl(t, slowEntered)
	pause := receiveControl(t, ticket)
	select {
	case <-pause.(*pauseRequest).done:
		t.Fatal("pause acknowledged with a worker still running")
	default:
	}
	close(release)
	if err := pause.Wait(controlContext(t)); err != nil {
		t.Fatal(err)
	}
	if err := e.Continue(); err != nil {
		t.Fatal(err)
	}
	if err := receiveControl(t, done); err != nil {
		t.Fatal(err)
	}
	t.Log("parallel pause waited for both handlers in the active batch")
}

func TestControlsRaceWithRunStartAndCompletion(t *testing.T) {
	controlEngines(t, func(t *testing.T, e testEngine) {
		t.Helper()
		ctx := controlContext(t)
		var handled int
		e.RegisterHandler("model", panicHandler(func(Event) { handled++ }))
		for i := 1; i <= 100; i++ {
			e.Schedule(EventBase{Time_: VTimeInPicoSec(i), HandlerID_: "model"})
			done := startControlRun(e)
			if err := e.RequestPause().Wait(ctx); err != nil {
				t.Fatal(err)
			}
			if err := e.Inspect(ctx, func() error {
				if handled < i-1 || handled > i {
					t.Errorf("invalid snapshot count: %d", handled)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if err := e.Continue(); err != nil {
				t.Fatal(err)
			}
			if err := receiveControl(t, done); err != nil {
				t.Fatal(err)
			}
		}
		if handled != 100 {
			t.Fatal("lost events while controlling start/end transitions")
		}
	})
}

func requireControlSuccess(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func assertPausedSnapshot(t *testing.T, e Engine, events *int) {
	t.Helper()
	requireControlSuccess(t, e.Inspect(controlContext(t), func() error {
		if *events != 1 || e.CurrentTime() != 1 {
			t.Errorf("snapshot: events=%d time=%d", *events, e.CurrentTime())
		}
		return nil
	}))
	if !e.IsPaused() {
		t.Fatal("inspection resumed a paused engine")
	}
}
