package httpapi

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/stretchr/testify/require"
)

func TestTraceReaderUsesLiteralRecordingPath(t *testing.T) {
	for _, readOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "offline", true: "live"}[readOnly], func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "trace #?.sqlite3")
			type entry struct{ Value int }
			writer := datarecording.NewDataRecorder(path)
			writer.CreateTable("literal_path", entry{})
			writer.InsertData("literal_path", entry{Value: 42})
			require.NoError(t, writer.Close())
			reader := NewSQLiteTraceReader(path)
			if readOnly {
				reader.InitReadOnly()
			} else {
				reader.Init()
			}
			t.Cleanup(func() { _ = reader.Close() })
			var value int
			require.NoError(t, reader.QueryRow("SELECT Value FROM literal_path").Scan(&value))
			require.Equal(t, 42, value)
			if readOnly {
				_, err := reader.Exec("INSERT INTO literal_path VALUES (43)")
				require.Error(t, err, "live reader must remain read-only")
			}
			require.NoError(t, reader.Close())
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, files, 1, "reader must not create another database")
			require.Equal(t, filepath.Base(path), files[0].Name())
		})
	}
}
