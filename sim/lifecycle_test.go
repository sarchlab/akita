package sim

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lifecycleHandler func(timing.Event)

func (h lifecycleHandler) Handle(e timing.Event) { h(e) }

// Exercise both wrapper and direct calls; paused execution still owns state.
func TestLifecycleRejectsActiveExecution(t *testing.T) {
	for _, mode := range []string{"simulation", "direct", "until", "parallel"} {
		t.Run(mode, func(t *testing.T) {
			b := MakeBuilder()
			if mode == "parallel" {
				b = b.WithParallelEngine()
			}
			s := b.Build()
			path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
			require.NoError(t, os.WriteFile(path, []byte("unchanged"), 0600))
			entered, release := make(chan struct{}), make(chan struct{})
			e := s.Engine()
			e.RegisterHandler("handler", lifecycleHandler(func(evt timing.Event) {
				if evt.Time() != 1 {
					return
				}
				// The same rejection must work from within an event handler, without deadlock.
				assert.ErrorContains(t, s.SaveCheckpoint(path), "execution is active")
				assert.ErrorContains(t, s.Terminate(), "execution is active")
				close(entered)
				<-release
			}))
			require.NoError(t, s.Initialize())
			e.Schedule(timing.MakeEventBase(s.NewID(), 1, "handler"))
			e.Schedule(timing.MakeEventBase(s.NewID(), 2, "handler"))
			run := s.Run
			if mode == "direct" || mode == "parallel" {
				run = e.Run
			}
			if mode == "until" {
				run = func() error { return e.(*timing.SerialEngine).RunUntil(2) }
			}
			done := make(chan error, 1)
			go func() { done <- run() }()
			<-entered
			require.ErrorContains(t, s.SaveCheckpoint(path), "execution is active")
			require.ErrorContains(t, s.LoadCheckpoint(path), "execution is active")
			require.ErrorContains(t, s.Terminate(), "execution is active")
			pause := e.RequestPause()
			close(release)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			require.NoError(t, pause.Wait(ctx))
			require.True(t, e.State().IsPaused())
			require.ErrorContains(t, s.SaveCheckpoint(path), "execution is active")
			require.ErrorContains(t, s.LoadCheckpoint(path), "execution is active")
			require.ErrorContains(t, s.Terminate(), "execution is active")
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, "unchanged", string(data))
			require.NoFileExists(t, path+".tmp")
			require.NoError(t, e.Continue())
			require.NoError(t, <-done)
			if mode != "parallel" {
				require.NoError(t, s.SaveCheckpoint(path))
				require.NoError(t, s.LoadCheckpoint(path))
			}
			assertTerminated(t, s, path)
		})
	}
}

type blockingSnapshot struct{ entered, release chan struct{} }

func (*blockingSnapshot) Name() string { return "Resource" }
func (r *blockingSnapshot) SaveCheckpoint(w io.Writer) error {
	close(r.entered)
	<-r.release
	_, err := io.WriteString(w, "state")
	return err
}
func (*blockingSnapshot) LoadCheckpoint(io.Reader) error { return nil }

func TestCheckpointExcludesDirectEngineStartup(t *testing.T) {
	s := MakeBuilder().Build()
	r := &blockingSnapshot{make(chan struct{}), make(chan struct{})}
	s.RegisterResource(r)
	handled := make(chan struct{})
	s.Engine().RegisterHandler("handler", lifecycleHandler(func(timing.Event) { close(handled) }))
	require.NoError(t, s.Initialize())
	s.Engine().Schedule(timing.MakeEventBase(s.NewID(), 1, "handler"))
	saved := make(chan error, 1)
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	go func() { saved <- s.SaveCheckpoint(path) }()
	<-r.entered
	started, done := make(chan struct{}), make(chan error, 1)
	go func() { close(started); done <- s.Engine().Run() }()
	<-started
	select {
	case <-handled:
		t.Fatal("engine dispatched while checkpoint was in progress")
	case <-time.After(25 * time.Millisecond):
	}
	close(r.release)
	require.NoError(t, <-saved)
	require.NoError(t, <-done)
	<-handled
	require.NoError(t, s.Terminate())
}

type closeProbe struct {
	datarecording.DataRecorder
	calls        int
	err          error
	panicOnClose bool
}

func (r *closeProbe) Close() error {
	r.calls++
	if r.panicOnClose {
		panic("close failed")
	}
	return r.err
}

func TestTerminationRetainsCleanupFailureOnce(t *testing.T) {
	for _, panics := range []bool{false, true} {
		t.Run(map[bool]string{false: "error", true: "panic"}[panics], func(t *testing.T) {
			s := MakeBuilder().Build()
			probe := &closeProbe{err: errors.New("close failed"), panicOnClose: panics}
			s.dataRecorder = probe
			results := make(chan error, 8)
			var wg sync.WaitGroup
			for range 8 {
				wg.Add(1)
				go func() { defer wg.Done(); results <- s.Terminate() }()
			}
			wg.Wait()
			close(results)
			first := <-results
			require.ErrorContains(t, first, "close failed")
			for err := range results {
				require.Same(t, first, err)
			}
			require.Equal(t, 1, probe.calls)
			require.ErrorContains(t, s.Initialize(), "terminated")
			require.ErrorContains(t, s.Run(), "terminated")
			require.ErrorContains(t, s.SaveCheckpoint("unused"), "terminated")
			require.ErrorContains(t, s.LoadCheckpoint("unused"), "terminated")
		})
	}
}

func assertTerminated(t *testing.T, s *Simulation, path string) {
	t.Helper()
	require.NoError(t, s.Terminate())
	require.NoError(t, s.Terminate())
	require.ErrorContains(t, s.Run(), "terminated")
	require.ErrorContains(t, s.SaveCheckpoint(path), "terminated")
	require.ErrorContains(t, s.LoadCheckpoint(path), "terminated")
}

type lifecycleInitializer struct {
	s      *Simulation
	errors []error
}

func (*lifecycleInitializer) Name() string         { return "Initializer" }
func (*lifecycleInitializer) ValidateSetup() error { return nil }
func (p *lifecycleInitializer) InitializeComponent() {
	p.errors = []error{p.s.Run(), p.s.SaveCheckpoint("unused"),
		p.s.LoadCheckpoint("unused"), p.s.Terminate(), p.s.Initialize()}
}
func TestInitializerLifecycleCallsAreRejectedWithoutBlocking(t *testing.T) {
	s := MakeBuilder().Build()
	probe := &lifecycleInitializer{s: s}
	s.RegisterResource(probe)
	require.NoError(t, s.Initialize())
	require.Len(t, probe.errors, 5)
	for _, err := range probe.errors {
		require.ErrorContains(t, err, "initialization is active")
	}
	require.NoError(t, s.Run())
	require.NoError(t, s.Terminate())
}
