package datarecording

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

type concurrentRecord struct {
	ID       int
	Location string `akita_data:"location"`
}

func TestConcurrentRecording(t *testing.T) {
	for _, manualFlush := range []bool{false, true} {
		t.Run(fmt.Sprintf("manual_flush=%t", manualFlush), func(t *testing.T) {
			db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "records.sqlite3"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = db.Close() })
			recorder := NewDataRecorderWithDB(db).(*sqliteWriter)
			recorder.batchSize = 32
			recorder.CreateTable("first", concurrentRecord{})
			recorder.CreateTable("second", concurrentRecord{})

			const writers, perWriter = 4, 400
			start := make(chan struct{})
			var workers sync.WaitGroup
			for writer := range writers {
				workers.Go(func() {
					<-start
					insertConcurrentRecords(recorder, writer*perWriter, perWriter)
				})
			}
			if manualFlush {
				for range 2 {
					workers.Go(func() {
						<-start
						for range 100 {
							recorder.Flush()
							runtime.Gosched()
						}
					})
				}
			}
			close(start)
			workers.Wait()
			recorder.Flush()
			recorder.Flush() // An empty flush must not replay the last batch.

			assertConcurrentRecords(t, db, writers*perWriter)
			if err := recorder.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func insertConcurrentRecords(recorder DataRecorder, firstID, count int) {
	for i := range count {
		id := firstID + i
		tableName := "first"
		if id%2 != 0 {
			tableName = "second"
		}
		recorder.InsertData(tableName, concurrentRecord{
			ID: id, Location: fmt.Sprintf("location-%d", id%7),
		})
		runtime.Gosched()
	}
}

func assertConcurrentRecords(t *testing.T, db *sql.DB, expected int) {
	t.Helper()
	rows, err := db.Query(`
		SELECT records.ID, location.Locale FROM (
			SELECT ID, Location FROM first UNION ALL SELECT ID, Location FROM second
		) AS records LEFT JOIN location ON records.Location = location.ID`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := make(map[int]bool, expected)
	for rows.Next() {
		var id int
		var location string
		if err := rows.Scan(&id, &location); err != nil {
			t.Fatal(err)
		}
		if id < 0 || id >= expected || seen[id] {
			t.Fatalf("unexpected or duplicate ID %d", id)
		}
		if want := fmt.Sprintf("location-%d", id%7); location != want {
			t.Fatalf("ID %d: location = %q, want %q", id, location, want)
		}
		seen[id] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(seen) != expected {
		t.Fatalf("persisted %d distinct records, want %d", len(seen), expected)
	}
	t.Logf("Persisted all %d records exactly once with correct locations", expected)
}

func TestFlushRollsBackAndRetainsBatch(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "rollback.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	recorder := NewDataRecorderWithDB(db)
	recorder.CreateTable("first", concurrentRecord{})
	recorder.CreateTable("second", concurrentRecord{})
	_, err = db.Exec(`CREATE TRIGGER reject_row BEFORE INSERT ON first
		WHEN NEW.ID = 1 BEGIN SELECT RAISE(ABORT, 'test insert failure'); END`)
	if err != nil {
		t.Fatal(err)
	}
	for id := range 2 {
		recorder.InsertData("first", concurrentRecord{
			ID: id, Location: fmt.Sprintf("location-%d", id),
		})
	}
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Flush did not report the injected insert failure")
			}
		}()
		recorder.Flush()
	}()
	var count int
	if err := db.QueryRow("SELECT COUNT(*) FROM first").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("failed flush committed %d rows", count)
	}
	if _, err := db.Exec("DROP TRIGGER reject_row"); err != nil {
		t.Fatal(err)
	}
	recorder.Flush()
	assertConcurrentRecords(t, db, 2)
	if err := recorder.Close(); err != nil {
		t.Fatal(err)
	}
}
