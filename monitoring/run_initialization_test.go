package monitoring

import (
	"net/http/httptest"
	"path/filepath"
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
	require.Nil(t, m.httpServer, "uninitialized simulation must not serve monitor requests")
	require.Nil(t, m.runSimulation)
	require.Error(t, s.Run())
	require.Empty(t, handled)
	require.NoError(t, s.Initialize())
	m.run(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/run", nil))
	select {
	case <-handled:
	case <-time.After(5 * time.Second):
		t.Fatal("initialized monitor did not run queued event")
	}
	t.Log("No monitor listener existed before Initialize; POST /api/run executed the queued event after startup")
}
