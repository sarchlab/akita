package monitoring

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"
	"github.com/stretchr/testify/require"
)

func TestExecutionInfoUsesLiteralRecordingPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "live #?.sqlite3")
	m := MakeBuilder().Build()
	s := sim.MakeBuilder().WithOutputPath(path).WithMonitor(m).Build()
	terminated := false
	t.Cleanup(func() {
		if !terminated {
			s.Terminate()
		}
	})
	rsp := httptest.NewRecorder()
	m.apiExecutionInfo(rsp, httptest.NewRequest("GET", "/", nil))
	require.Equal(t, 200, rsp.Code)
	if !strings.Contains(rsp.Body.String(), "Start Time") {
		t.Errorf("execution info did not read the recording: %s", rsp.Body)
	}
	s.Terminate()
	terminated = true
	files, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, files, 1, "reader must not create another database")
	require.Equal(t, filepath.Base(path), files[0].Name())
}
