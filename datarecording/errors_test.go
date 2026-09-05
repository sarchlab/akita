package datarecording

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"
)

func TestWriterOperationalFailuresReturnErrors(t *testing.T) {
	file := filepath.Join(t.TempDir(), "recording")
	recorder, err := NewDataRecorder(file)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDataRecorder(file); err == nil {
		t.Fatal("second recorder replaced owned output")
	}
	w := recorder.(*sqliteWriter)
	if err := w.CreateTable("records", struct{ ID int }{}); err != nil {
		t.Fatal(err)
	}
	if err := w.InsertData("records", struct{ ID int }{1}); err != nil {
		t.Fatal(err)
	}
	if err := w.DB.Close(); err != nil {
		t.Fatal(err)
	}
	if err := w.Flush(); err == nil {
		t.Fatal("flush hid closed database")
	}
	if w.entryCount != 1 {
		t.Fatal("failed flush lost retained batch")
	}
	if err := w.Close(); err == nil {
		t.Fatal("final flush failure hidden by close")
	}
	if _, err := NewDataRecorder(filepath.Join(t.TempDir(), "missing", "recording")); err == nil {
		t.Fatal("bad output path accepted")
	}
}
func TestReaderScanAndCancellationReturnErrors(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE records (ID TEXT); INSERT INTO records VALUES ('not-an-integer')"); err != nil {
		t.Fatal(err)
	}
	reader := NewReaderWithDB(db)
	reader.MapTable("records", struct{ ID int }{})
	rows, count, err := reader.Query(context.Background(), "records", QueryParams{})
	if err == nil || rows != nil || count != 0 {
		t.Fatalf("scan failure became partial results: %v %d %v", rows, count, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := reader.Query(ctx, "records", QueryParams{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation lost: %v", err)
	}
}

func TestClosedRecorderRejectsWrites(t *testing.T) {
	r, err := NewDataRecorder(filepath.Join(t.TempDir(), "result"))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.CreateTable("records", struct{ ID int }{}); err != nil {
		t.Fatal(err)
	}
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	for _, err := range []error{
		r.InsertData("records", struct{ ID int }{1}),
		r.CreateTable("more", struct{ ID int }{}), r.Flush(),
	} {
		if !errors.Is(err, sql.ErrConnDone) {
			t.Fatalf("closed recorder accepted an operation: %v", err)
		}
	}
}

func TestOutputPathsRemainLiteralSQLiteFilenames(t *testing.T) {
	base := filepath.Join(t.TempDir(), "result")
	type record struct{ ID int }
	for i, name := range []string{base, base + ".sqlite3?ignored#suffix"} {
		r, err := NewDataRecorder(name)
		if err != nil {
			t.Fatal(err)
		}
		if err := r.CreateTable("records", record{}); err != nil {
			t.Fatal(err)
		}
		if err := r.InsertData("records", record{ID: i}); err != nil {
			t.Fatal(err)
		}
		if err := r.Close(); err != nil {
			t.Fatal(err)
		}
		reader, err := NewReader(name + ".sqlite3")
		if err != nil {
			t.Fatal(err)
		}
		reader.MapTable("records", record{})
		rows, count, err := reader.Query(context.Background(), "records", QueryParams{})
		if err != nil || count != 1 || len(rows) != 1 || rows[0].(*record).ID != i {
			t.Fatalf("output path aliased another recording: rows=%v count=%d err=%v", rows, count, err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
	}
}
