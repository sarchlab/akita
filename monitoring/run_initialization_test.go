package monitoring

import (
	"log"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/timing"
	"github.com/stretchr/testify/require"
)

type monitoredRunEvents chan timing.Event

func (c monitoredRunEvents) Handle(e timing.Event) { c <- e }

func TestMonitorRunRequiresInitialization(t *testing.T) {
	m := MakeBuilder().Build()
	s := sim.MakeBuilder().WithOutputPath(filepath.Join(t.TempDir(), "live.sqlite3")).WithMonitor(m).Build()
	defer s.Terminate()
	handled := make(monitoredRunEvents, 1)
	s.Engine().RegisterHandler("probe", handled)
	s.Engine().Schedule(timing.EventBase{At: 1, Handler: "probe"})
	messages := make(runLog, 1)
	previous := log.Writer()
	log.SetOutput(messages)
	defer log.SetOutput(previous)
	m.run(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/run", nil))
	select {
	case message := <-messages:
		require.True(t, strings.Contains(message, "Initialize"), message)
	case <-handled:
		t.Fatal("monitor ran a queued event before initialization")
	case <-time.After(5 * time.Second):
		t.Fatal("monitor did not report the initialization error")
	}
	require.NoError(t, s.Initialize())
	require.NoError(t, s.Run())
	require.Len(t, handled, 1, "rejected monitor run must leave the event queued")
	t.Log("POST /api/run reported Initialize and left the event queued; it executed once after initialization")
}
