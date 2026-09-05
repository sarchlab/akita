package timing

import (
	"testing"
	"testing/synctest"

	"github.com/sarchlab/akita/v5/hooking"
)

func TestSerialPauseAcknowledgesCompletedCallbacks(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := NewSerialEngine()
		entered := make(chan struct{})
		release := make(chan struct{})
		count, afterHooks := 0, 0
		e.RegisterHandler("model", failureHandler(func(evt Event) {
			entered <- struct{}{}
			<-release
			count++
			if evt.Time() < 4 {
				e.Schedule(MakeEventBase(e.IDGenerator(), evt.Time()+1, "model"))
			}
		}))
		hook := failureHook(func(ctx hooking.HookCtx) {
			if ctx.Pos == HookPosAfterEvent {
				afterHooks++
			}
		})
		e.AcceptHook(&hook)
		e.Schedule(MakeEventBase(e.IDGenerator(), 1, "model"))
		done := make(chan error, 1)
		go func() { done <- e.Run() }()
		for want := 1; want <= 4; want++ {
			<-entered
			paused := make(chan struct{})
			go func() { e.Pause(); close(paused) }()
			synctest.Wait()
			select {
			case <-paused:
				t.Fatal("Pause returned before the callback finished")
			default:
			}
			release <- struct{}{}
			<-paused
			// Deliberately read ordinary model fields: the acknowledgment must
			// synchronize both the handler and its after-event hook with inspection.
			if count != want || afterHooks != want || e.CurrentTime() != VTimeInPicoSec(want) {
				t.Fatalf("incomplete paused state: events=%d hooks=%d time=%d", count, afterHooks, e.CurrentTime())
			}
			select {
			case <-entered:
				t.Fatal("next callback entered while paused")
			default:
			}
			e.Continue()
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		t.Logf("four pauses observed completed handlers and hooks; all four events finished after resume")
	})
}

func TestSerialPauseBeforeAndBetweenRuns(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := NewSerialEngine()
		count := 0
		e.RegisterHandler("model", failureHandler(func(Event) { count++ }))
		for i := 1; i <= 3; i++ {
			e.Schedule(MakeEventBase(e.IDGenerator(), VTimeInPicoSec(i), "model"))
			e.Pause()
			done := make(chan error, 1)
			go func() { done <- e.RunUntil(VTimeInPicoSec(i)) }()
			synctest.Wait()
			if count != i-1 {
				t.Fatal("run dispatched despite an existing pause")
			}
			// A second caller must observe the same acknowledged pause.
			e.Pause()
			e.Continue()
			if err := <-done; err != nil {
				t.Fatal(err)
			}
			if count != i {
				t.Fatalf("resume did not finish run %d: %d events", i, count)
			}
		}
	})
}
