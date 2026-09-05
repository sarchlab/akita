package simulation

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeIsolated(t *testing.T, parallel bool, setup ...func(*Simulation) error) (*Simulation, error) {
	t.Helper()
	b := MakeBuilder().WithoutMonitoring().WithoutSourceRecording().
		WithOutputFileName(filepath.Join(t.TempDir(), "result"))
	if parallel {
		b = b.WithParallelEngine()
	}
	return b.Build(setup...)
}

func TestSetupFailureDoesNotAffectAnotherSimulation(t *testing.T) {
	for _, parallel := range []bool{false, true} {
		good, err := makeIsolated(t, parallel)
		if err != nil {
			t.Fatal(err)
		}
		before := good.GetIDGenerator().Generate()
		bad, err := makeIsolated(t, parallel, func(s *Simulation) error {
			s.GetIDGenerator().Generate()
			panic("invalid setup")
		})
		if err == nil || bad.Err() == nil {
			t.Fatal("setup panic escaped failure boundary")
		}
		if next := good.GetIDGenerator().Generate(); next != before+1 {
			t.Fatalf("another simulation changed IDs: %d -> %d", before, next)
		}
		if err := good.Run(); err != nil {
			t.Fatal(err)
		}
		if err := good.Terminate(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(good.outputPath + ".complete"); err != nil {
			t.Fatal("successful output is not complete", err)
		}
		if _, err := os.Stat(bad.outputPath + ".complete"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed output marked complete")
		}
	}
}

type failingRecorder struct {
	closeErr   error
	closePanic any
	closed     bool
}

func (*failingRecorder) CreateTable(string, any) error { return nil }
func (*failingRecorder) InsertData(string, any) error  { return nil }
func (*failingRecorder) ListTables() []string          { return nil }
func (*failingRecorder) Flush() error                  { return nil }
func (r *failingRecorder) Close() error {
	r.closed = true
	if r.closePanic != nil {
		panic(r.closePanic)
	}
	return r.closeErr
}

func TestFinalizationFailureIsContainedAndNeverComplete(t *testing.T) {
	sentinel := errors.New("close failed")
	for _, panicValue := range []any{nil, "cleanup panic"} {
		good, err := makeIsolated(t, false)
		if err != nil {
			t.Fatal(err)
		}
		bad, err := makeIsolated(t, false)
		if err != nil {
			t.Fatal(err)
		}
		if err := bad.dataRecorder.Close(); err != nil {
			t.Fatal(err)
		}
		fake := &failingRecorder{closeErr: sentinel, closePanic: panicValue}
		bad.dataRecorder = fake
		bad.metaRecorder = nil
		bad.topologyRecorder = nil
		bad.visTracer = nil
		if err := bad.Terminate(); err == nil || !fake.closed {
			t.Fatalf("finalization failure lost: %v", err)
		}
		if _, err := os.Stat(bad.outputPath + ".complete"); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("failed output marked complete")
		}
		if err := good.Terminate(); err != nil {
			t.Fatal("another simulation failed", err)
		}
	}
}

type restoreProbe struct {
	value int
	fail  bool
}

func (*restoreProbe) Name() string { return "probe" }
func (p *restoreProbe) SaveCheckpoint(w io.Writer) error {
	_, err := w.Write([]byte{byte(p.value)})
	return err
}
func (p *restoreProbe) LoadCheckpoint(r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	p.value = int(b[0])
	if p.fail {
		return errors.New("bad checkpoint component")
	}
	return nil
}
func TestRestoreFailureDiscardsOnlyFreshTarget(t *testing.T) {
	source, err := makeIsolated(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer source.Terminate()
	source.RegisterResource(&restoreProbe{value: 7})
	for i := 0; i < 10; i++ {
		source.GetIDGenerator().Generate()
	}
	file := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	if err := source.SaveCheckpoint(file, "test"); err != nil {
		t.Fatal(err)
	}
	bad, err := makeIsolated(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Terminate()
	probe := &restoreProbe{fail: true}
	bad.RegisterResource(probe)
	if err := bad.LoadCheckpoint(file, "test"); err == nil {
		t.Fatal("bad restore succeeded")
	}
	if probe.value != 7 {
		t.Fatal("test did not exercise a partially applied target")
	}
	if bad.Run() == nil || bad.SaveCheckpoint(file+".invalid", "test") == nil || bad.LoadCheckpoint(file, "test") == nil {
		t.Fatal("failed target reused")
	}
	if source.GetIDGenerator().Generate() != 11 {
		t.Fatal("restoring target changed source ID sequence")
	}
	fresh, err := makeIsolated(t, false)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Terminate()
	fresh.RegisterResource(&restoreProbe{})
	if err := fresh.LoadCheckpoint(file, "test"); err != nil {
		t.Fatal(err)
	}
	if fresh.GetIDGenerator().Generate() != 11 || source.GetIDGenerator().Generate() != 12 {
		t.Fatal("restored sequences are not independent")
	}
	if err := fresh.Run(); err != nil {
		t.Fatal(err)
	}
	if err := fresh.LoadCheckpoint(file, "test"); err == nil {
		t.Fatal("restored an executed target")
	}
}

func TestOptionalCapabilitiesDegradeWithDiagnostics(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	s, err := MakeBuilder().WithMonitorPort(port).WithVisTracingOnStart().
		WithSourceFS("broken", unavailableSourceFS{}).
		WithOutputFileName(filepath.Join(t.TempDir(), "result")).Build()
	if err != nil {
		t.Fatal("optional monitor failure invalidated setup", err)
	}
	defer s.Terminate()
	if s.GetMonitor() != nil || len(s.Warnings()) != 2 {
		t.Fatalf("optional failures were not both reported: %v", s.Warnings())
	}
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
}

func TestFailureDiagnosticsAreIndependentSnapshots(t *testing.T) {
	s, err := makeIsolated(t, false, func(*Simulation) error { return errors.New("setup") })
	if err == nil {
		t.Fatal("expected failure")
	}
	first := s.Failures()[0]
	original := append([]byte(nil), first.Stack...)
	first.Operation = "changed"
	first.Stack[0] ^= 255
	if s.Failures()[0].Operation == "changed" || !bytes.Equal(s.Failures()[0].Stack, original) {
		t.Fatal("caller mutated diagnostics")
	}
	if !strings.Contains(s.Err().Error(), s.ID()) {
		t.Fatal("simulation identity missing")
	}
}

// The root itself cannot be opened, so ArchiveFS must surface an I/O error.
type unavailableSourceFS struct{}

func (unavailableSourceFS) Open(string) (fs.File, error) {
	return nil, fs.ErrPermission
}

func TestMonitorIsPublishedOnlyAfterManagedSetup(t *testing.T) {
	var configured bool
	s, err := MakeBuilder().WithoutSourceRecording().
		WithOutputFileName(filepath.Join(t.TempDir(), "result")).
		Build(func(s *Simulation) error {
			if s.GetMonitor() != nil {
				t.Fatal("monitor exposed partially constructed components")
			}
			configured = true
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Terminate()
	if !configured || s.GetMonitor() == nil {
		t.Fatal("monitor not published after setup")
	}
}
