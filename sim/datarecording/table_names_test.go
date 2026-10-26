package datarecording

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/sarchlab/akita/v5/internal/sqlitepath"
)

type countRecord struct{ ID int }

// TestCreateTableRejectsExistingName checks that creating a table the recording
// already has, in any ASCII case as SQLite compares identifiers, panics with a
// message naming the existing table instead of a SQLite error. Names that differ
// only in non-ASCII case are distinct to SQLite and are accepted. The recorder
// stays usable afterwards.
func TestCreateTableRejectsExistingName(t *testing.T) {
	path := filepath.Join(t.TempDir(), "names.sqlite3")
	recorder := NewDataRecorder(path)
	recorder.CreateTable("results", countRecord{})
	recorder.CreateTable("tagged", concurrentRecord{}) // creates akita$location
	recorder.CreateTable("café", countRecord{})
	recorder.CreateTable("CAFÉ", countRecord{})

	for name, existing := range map[string]string{
		"results":        "results",
		"RESULTS":        "results",
		"Tagged":         "tagged",
		"AKITA$Location": "akita$location",
		"Café":           "café",
	} {
		want := fmt.Sprintf("datarecording: table %q already exists in this recording", existing)
		if got := recoverPanic(func() { recorder.CreateTable(name, countRecord{}) }); got != want {
			t.Errorf("CreateTable(%q) panic = %v, want %q", name, got, want)
		}
	}

	recorder.InsertData("results", countRecord{ID: 1})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := openReadOnly(t, path).QueryRow("SELECT COUNT(*) FROM results").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("results rows = %d, want 1", count)
	}
}

// TestApplicationTablesMayUseGenericNames checks that application tables named
// location and trace coexist with the interning dictionary, which lives in
// akita$location, even when the application's location table comes first.
func TestApplicationTablesMayUseGenericNames(t *testing.T) {
	type place struct{ Name string }

	path := filepath.Join(t.TempDir(), "generic.sqlite3")
	recorder := NewDataRecorder(path)
	recorder.CreateTable("location", place{})
	recorder.CreateTable("trace", concurrentRecord{})
	recorder.InsertData("location", place{Name: "lab"})
	recorder.InsertData("trace", concurrentRecord{ID: 7, Location: "GPU.L1"})
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}

	db := openReadOnly(t, path)
	var id int
	var locale string
	if err := db.QueryRow(`SELECT t.ID, l.Locale FROM trace t
		JOIN akita$location l ON t.Location = l.ID`).Scan(&id, &locale); err != nil {
		t.Fatal(err)
	}
	if id != 7 || locale != "GPU.L1" {
		t.Fatalf("trace row = (%d, %q), want (7, %q)", id, locale, "GPU.L1")
	}

	var name string
	if err := db.QueryRow("SELECT Name FROM location").Scan(&name); err != nil {
		t.Fatal(err)
	}
	if name != "lab" {
		t.Fatalf("location row = %q, want %q", name, "lab")
	}
}

func recoverPanic(f func()) (recovered any) {
	defer func() { recovered = recover() }()
	f()

	return nil
}

func openReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", sqlitepath.URI(path, "ro"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}
