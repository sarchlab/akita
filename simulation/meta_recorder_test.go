package simulation

import (
	"context"
	"os"
	"testing"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/timing"
)

type fakeTimeTeller struct {
	now timing.VTimeInPicoSec
}

func (f *fakeTimeTeller) CurrentTime() timing.VTimeInPicoSec {
	return f.now
}

func TestMetaRecorderWritesExecInfo(t *testing.T) {
	path := "test_meta_recorder"
	dbFile := path + ".sqlite3"

	os.Remove(dbFile)
	defer os.Remove(dbFile)

	clock := &fakeTimeTeller{now: 10}
	recorder := datarecording.NewDataRecorder(path)
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

func readExecInfo(t *testing.T, dbFile string) map[string]string {
	t.Helper()

	reader := datarecording.NewReader(dbFile)
	defer reader.Close()
	reader.MapTable("exec_info", simulationInfo{})

	results, _, err := reader.Query(
		context.Background(), "exec_info", datarecording.QueryParams{})
	if err != nil {
		t.Fatalf("query exec_info: %v", err)
	}

	values := make(map[string]string)
	for _, result := range results {
		info, ok := result.(*simulationInfo)
		if !ok {
			t.Fatalf("unexpected result type %T", result)
		}
		values[info.Property] = info.Value
	}

	return values
}
