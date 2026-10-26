package sim

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/sqlitepath"

	"github.com/sarchlab/akita/v5/sim/datarecording"
	"github.com/sarchlab/akita/v5/sim/timing"
)

type fakeTimeTeller struct {
	now timing.VTimeInPicoSec
}

func (f *fakeTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return f.now
}

func TestMetaRecorderWritesExecInfo(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "meta.sqlite3")

	clock := &fakeTimeTeller{now: 10}
	recorder := datarecording.NewDataRecorder(dbFile)
	metaRecorder := newMetaRecorder(recorder, clock)

	values := readExecInfo(t, dbFile)
	if values["Command"] == "" {
		t.Fatal("expected Command to be recorded at start")
	}
	if values["Working Directory"] == "" {
		t.Fatal("expected Working Directory to be recorded at start")
	}
	if values["Start Virtual Time"] != "10" {
		t.Fatalf("unexpected start virtual time %q", values["Start Virtual Time"])
	}

	clock.now = 123
	metaRecorder.End()
	if err := recorder.Close(); err != nil {
		t.Fatalf("close recorder: %v", err)
	}

	values = readExecInfo(t, dbFile)
	if values["End Virtual Time"] != "123" {
		t.Fatalf("unexpected end virtual time %q", values["End Virtual Time"])
	}
}

func openRecordingDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", sqlitepath.URI(path, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func readExecInfo(t *testing.T, dbFile string) map[string]string {
	t.Helper()
	rows, err := openRecordingDB(t, dbFile).Query("SELECT Property, Value FROM akita$exec_info")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	values := make(map[string]string)
	for rows.Next() {
		var property, value string
		if err := rows.Scan(&property, &value); err != nil {
			t.Fatal(err)
		}
		values[property] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return values
}
