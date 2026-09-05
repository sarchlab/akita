package simulation

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type shutdownInspectionProbe struct {
	block   atomic.Bool
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (p *shutdownInspectionProbe) Name() string {
	if p.block.Load() {
		p.once.Do(func() { close(p.entered) })
		<-p.release
	}
	return "shutdown-probe"
}

func TestHealthyOutputCompletesWithAnInflightMonitorInspection(t *testing.T) {
	s, probe, url := buildShutdownInspection(t)
	var release sync.Once
	unblock := func() { release.Do(func() { close(probe.release) }) }
	defer func() { unblock(); _ = s.Terminate() }()
	if err := s.Run(); err != nil {
		t.Fatal(err)
	}
	probe.block.Store(true)
	requestDone := make(chan struct{})
	go func() {
		defer close(requestDone)
		client := &http.Client{Timeout: 2 * time.Second}
		if response, err := client.Get(url); err == nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-probe.entered:
	case <-time.After(time.Second):
		t.Fatal("inspection did not reach the component")
	}
	terminated := make(chan error, 1)
	go func() { terminated <- s.Terminate() }()
	awaitClosedSimulation(t, s)
	unblock()
	if err := <-terminated; err != nil {
		t.Fatal("monitor inspection invalidated healthy output", err)
	}
	<-requestDone
	if _, err := os.Stat(s.outputPath + ".complete"); err != nil {
		t.Fatal("healthy output was not published", err)
	}
}

func buildShutdownInspection(t *testing.T) (*Simulation, *shutdownInspectionProbe, string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	probe := &shutdownInspectionProbe{entered: make(chan struct{}), release: make(chan struct{})}
	s, err := MakeBuilder().WithMonitorPort(port).WithoutSourceRecording().WithVisTracingOnStart().
		WithOutputFileName(filepath.Join(t.TempDir(), "result")).Build(func(s *Simulation) error {
		s.RegisterComponent(probe)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.GetMonitor() == nil {
		_ = s.Terminate()
		t.Fatal("monitor did not start")
	}
	return s, probe, fmt.Sprintf("http://127.0.0.1:%d/api/component/shutdown-probe", port)
}

func awaitClosedSimulation(t *testing.T, s *Simulation) {
	t.Helper()
	deadline := time.After(time.Second)
	for s.State() != "closed" {
		select {
		case <-deadline:
			t.Fatal("shutdown did not begin")
		default:
			time.Sleep(time.Millisecond)
		}
	}
}
