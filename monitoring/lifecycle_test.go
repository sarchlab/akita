package monitoring

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/simulation"
)

func TestSimulationAttachesLiveMonitor(t *testing.T) {
	output := filepath.Join(t.TempDir(), "live")
	monitor := NewMonitor()
	sim := simulation.MakeBuilder().WithOutputFileName(output).WithMonitor(monitor).Build()
	t.Cleanup(sim.Terminate)
	if monitor.simulation != sim || monitor.engine != sim.Engine() || monitor.visTracer == nil {
		t.Fatal("monitor was not attached to the simulation services")
	}
	if monitor.tracePath != output+".sqlite3" || monitor.httpServer == nil {
		t.Fatal("monitor was not started with the recording destination")
	}

	response := httptest.NewRecorder()
	monitor.httpServer.Handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/mode", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "live") {
		t.Fatalf("live endpoint: %d %s", response.Code, response.Body.String())
	}
}
