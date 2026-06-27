package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestEnsureIndexBuildsOnceAndInvalidatesDBInfo checks the on-demand index
// builder: a genuinely new build creates the index, records it, and drops the
// cached db_info overview (its on-disk size changed), while a repeat call or a
// pre-existing index is adopted without rebuilding or invalidating the cache.
func TestEnsureIndexBuildsOnceAndInvalidatesDBInfo(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "trace.sqlite3")
	reader := NewSQLiteTraceReader(dbPath)
	reader.Init()

	if _, err := reader.Exec(`CREATE TABLE trace (
		ID INTEGER, Location INTEGER, StartTime REAL, EndTime REAL, Kind TEXT, What TEXT)`); err != nil {
		t.Fatalf("create table: %v", err)
	}

	ctx := context.Background()
	const ddl = `CREATE INDEX IF NOT EXISTS idx_test_loc ON trace(Location, StartTime)`

	seedCache := func() {
		reader.dbInfo.mu.Lock()
		reader.dbInfo.info = &DBInfo{}
		reader.dbInfo.mu.Unlock()
	}
	cached := func() *DBInfo {
		reader.dbInfo.mu.Lock()
		defer reader.dbInfo.mu.Unlock()

		return reader.dbInfo.info
	}

	// First build: creates the index, records it, and invalidates the cache.
	seedCache()
	reader.ensureIndex(ctx, "Building index idx_test_loc", ddl)

	if !reader.indexExists(ctx, "idx_test_loc") {
		t.Fatal("index was not created")
	}
	if _, done := reader.builtIndexes.Load("idx_test_loc"); !done {
		t.Fatal("index not recorded in builtIndexes")
	}
	if cached() != nil {
		t.Fatal("db_info cache should be invalidated after a new index build")
	}

	// Repeat call: already in builtIndexes, so it must not invalidate again.
	seedCache()
	reader.ensureIndex(ctx, "Building index idx_test_loc", ddl)
	if cached() == nil {
		t.Fatal("db_info cache should survive a repeat ensureIndex of a built index")
	}

	// Fresh process: the index is persisted on disk but not yet in builtIndexes.
	// It must be adopted via the existence check, without invalidating db_info.
	reader.builtIndexes.Delete("idx_test_loc")
	seedCache()
	reader.ensureIndex(ctx, "Building index idx_test_loc", ddl)
	if _, done := reader.builtIndexes.Load("idx_test_loc"); !done {
		t.Fatal("pre-existing index not adopted into builtIndexes")
	}
	if cached() == nil {
		t.Fatal("db_info cache should survive adopting a pre-existing index")
	}
}

func TestIndexNameFromDDL(t *testing.T) {
	cases := map[string]string{
		`CREATE INDEX IF NOT EXISTS idx_a ON trace(Location)`: "idx_a",
		`CREATE INDEX idx_b ON trace(Location, StartTime)`:    "idx_b",
		"CREATE   INDEX\n  IF NOT EXISTS   idx_c ON t(x)":     "idx_c",
		`SELECT 1`: "",
	}
	for ddl, want := range cases {
		if got := indexNameFromDDL(ddl); got != want {
			t.Errorf("indexNameFromDDL(%q) = %q, want %q", ddl, got, want)
		}
	}
}

// TestDBFileBytesSumsSidecars checks that the reported on-disk footprint includes
// the WAL and shared-memory sidecars (where multi-GB index pages sit before a
// checkpoint) and skips any that are absent.
func TestDBFileBytesSumsSidecars(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, n int) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, make([]byte, n), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}

		return p
	}

	base := write("trace.sqlite3", 100)
	write("trace.sqlite3-wal", 4000)
	write("trace.sqlite3-shm", 32)
	if got, want := dbFileBytes(base), int64(100+4000+32); got != want {
		t.Fatalf("dbFileBytes with sidecars = %d, want %d", got, want)
	}

	soloBase := write("other.sqlite3", 7)
	if got, want := dbFileBytes(soloBase), int64(7); got != want {
		t.Fatalf("dbFileBytes without sidecars = %d, want %d", got, want)
	}
}
