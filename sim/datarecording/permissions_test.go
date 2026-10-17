package datarecording

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/sqlitepath"
)

func TestRecordingPreservesSQLiteFilePermissions(t *testing.T) {
	dir := t.TempDir()
	baselinePath := filepath.Join(dir, "sqlite-default.sqlite3")
	db, err := sql.Open("sqlite", sqlitepath.URI(baselinePath, "rwc"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec("CREATE TABLE baseline (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	baseline, err := os.Stat(baselinePath)
	if err != nil {
		t.Fatal(err)
	}

	recordingPath := filepath.Join(dir, "recording.sqlite3")
	recorder := NewDataRecorder(recordingPath)
	t.Cleanup(func() { _ = recorder.Close() })
	recorder.CreateTable("records", struct{ ID int }{})
	recording, err := os.Stat(recordingPath)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := recording.Mode().Perm(), baseline.Mode().Perm(); got != want {
		t.Fatalf("recording permissions = %04o; SQLite default = %04o", got, want)
	}
	t.Logf("Recording retained SQLite's default permissions: %04o", recording.Mode().Perm())
}
