package monitoring

import (
	"io"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
)

func TestSimulationAttachesLiveMonitor(t *testing.T) {
	output := filepath.Join(t.TempDir(), "live")
	var monitor *Monitor
	s := sim.MakeBuilder().WithOutputFileName(output).
		WithMonitorFactory(func(s *sim.Simulation) sim.Monitor {
			monitor = NewMonitor(s)
			if monitor.httpServer != nil {
				t.Fatal("constructor opened a listener")
			}
			return monitor
		}).Build()
	if monitor.simulation != s || monitor.engine != s.Engine() || monitor.visTracer != s.VisTracer() {
		t.Fatal("monitor was not bound to the simulation services")
	}
	if monitor.tracePath != output+".sqlite3" || monitor.httpServer == nil {
		t.Fatal("monitor was not started with the recording destination")
	}
	terminated := false
	t.Cleanup(func() {
		if !terminated {
			s.Terminate()
		}
	})
	_, port, err := net.SplitHostPort(monitor.httpServer.Addr)
	if err != nil {
		t.Fatal(err)
	}
	address := net.JoinHostPort("127.0.0.1", port)
	client := &http.Client{Timeout: 5 * time.Second}
	defer client.CloseIdleConnections()
	response, err := client.Get("http://" + address + "/api/mode")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil || response.StatusCode != http.StatusOK || !strings.Contains(string(body), "live") {
		t.Fatalf("live endpoint: %d %s, %v", response.StatusCode, body, err)
	}
	s.RegisterPort(messaging.NewPort("Agent.Top", 2, 2))
	if len(monitor.buffers) != 2 {
		t.Fatal("registered port buffers missing")
	}
	bar := monitor.CreateProgressBar("work", 10)
	if bar.ID == 0 || bar.Total != 10 {
		t.Fatal("progress bar was not initialized")
	}
	monitor.CompleteProgressBar(bar)
	s.Terminate()
	terminated = true
	connection, err := net.DialTimeout("tcp", address, time.Second)
	if err == nil {
		connection.Close()
		t.Fatal("monitor listener survived termination")
	}
	t.Logf("GET /api/mode returned %s; port buffers and progress bar registered; Terminate closed listener", body)
}

func TestMonitorRejectsRepeatedStart(t *testing.T) {
	s := sim.MakeBuilder().WithOutputFileName(filepath.Join(t.TempDir(), "live")).Build()
	defer s.Terminate()
	m := NewMonitor(s)
	m.Start()
	defer m.Stop()
	server := m.httpServer
	assertRejected := func() {
		t.Helper()
		defer func() {
			if recover() != "monitoring: monitor already started" {
				t.Error("repeated Start was not rejected")
			}
			if m.httpServer != server {
				t.Error("repeated Start replaced the server")
			}
		}()
		m.Start()
	}
	assertRejected()
	m.Stop()
	assertRejected()
}
