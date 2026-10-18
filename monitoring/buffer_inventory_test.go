package monitoring

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/sarchlab/akita/v5/sim/messaging"
	"github.com/sarchlab/akita/v5/sim/messaging/twowaybuffered"
	"github.com/sarchlab/akita/v5/sim/modeling"
	"github.com/sarchlab/akita/v5/sim/modeling/wakeup"
	"github.com/sarchlab/akita/v5/sim/queueing"
	"github.com/stretchr/testify/require"
)

type inventoryState struct {
	Queue queueing.Buffer[int]   `json:"queue"`
	Banks []queueing.Buffer[int] `json:"banks"`
}

type inventoryComponent = wakeup.Component[struct{}, inventoryState, modeling.None, modeling.None, modeling.None]

func (s *inventoryState) Initialize(*inventoryComponent) {
	s.Queue = queueing.MakeBuffer[int](7)
	s.Queue.Push(1)
	s.Banks = []queueing.Buffer[int]{queueing.MakeBuffer[int](4)}
}

// The monitor must only use buffer observation on this custom port, never its
// transport methods (left unset to fail immediately if accidentally called).
type absentBufferPort struct{ messaging.Port }

func (absentBufferPort) Owner() messaging.PortOwner       { return nil }
func (absentBufferPort) Connection() messaging.Connection { return nil }

func (absentBufferPort) Name() string                      { return "Unbuffered" }
func (absentBufferPort) IncomingBuf() messaging.BufferView { return nil }
func (absentBufferPort) OutgoingBuf() messaging.BufferView { return nil }

func TestMonitorDiscoversInitializedStateAndAbsentBuffers(t *testing.T) {
	m := MakeBuilder().Build()
	s := sim.MakeBuilder().WithMonitor(m).
		WithOutputPath(filepath.Join(t.TempDir(), "monitor.sqlite3")).Build()
	defer s.Terminate()
	c := modeling.MakeBuilder[*inventoryComponent](struct{}{}).WithSimulation(s).Build("Comp")
	p := twowaybuffered.NewPort(2, 3)
	monitorPortOwner{}.BindPort("Port", p)
	s.RegisterPort(p)
	s.RegisterPort(absentBufferPort{})
	require.False(t, m.started)
	require.NoError(t, s.Initialize())

	read := func() map[string]bufferRsp {
		t.Helper()
		w := httptest.NewRecorder()
		m.hangDetectorBuffers(w, httptest.NewRequest("GET", "/api/hangdetector/buffers?limit=20", nil))
		require.Equal(t, 200, w.Code)
		var rows []bufferRsp
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &rows))
		result := make(map[string]bufferRsp)
		for _, row := range rows {
			result[row.Buffer] = row
		}
		return result
	}
	rows := read()
	require.Len(t, rows, 4, "absent buffers must be omitted while empty buffers remain visible")
	require.Equal(t, bufferRsp{Buffer: "Comp.State.Queue", Level: 1, Cap: 7}, rows["Comp.State.Queue"])
	require.Equal(t, 4, rows["Comp.State.Banks[0]"].Cap)
	require.Equal(t, 2, rows[p.Name()+".in"].Cap)
	require.Equal(t, 3, rows[p.Name()+".out"].Cap)
	// Checkpoint loading replaces State and its slice storage. Resolve the new
	// buffers instead of retaining pointers into the old backing array.
	c.State = inventoryState{Queue: queueing.MakeBuffer[int](7),
		Banks: []queueing.Buffer[int]{queueing.MakeBuffer[int](4)}}
	c.State.Banks[0].Push(42)
	rows = read()
	require.Zero(t, rows["Comp.State.Queue"].Level)
	require.Equal(t, 1, rows["Comp.State.Banks[0]"].Level)
	t.Log("Buffer API returned initialized State and bank buffers, real port capacities 2/3, " +
		"omitted absent buffers, and observed replaced State")
}
