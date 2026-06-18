package simulation

import (
	"database/sql"
	"encoding/base64"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/sarchlab/akita/v5/datarecording"
	"github.com/sarchlab/akita/v5/sourcefs"
)

// openMemRecorder returns a data recorder over a single-connection in-memory
// SQLite DB. MaxOpenConns(1) is required: a ":memory:" database is per
// connection, so the table the recorder creates must be read back on the same
// connection.
func openMemRecorder(t *testing.T) (*sql.DB, datarecording.DataRecorder) {
	t.Helper()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	return db, datarecording.NewDataRecorderWithDB(db)
}

func readSourceRows(t *testing.T, db *sql.DB) (content, format map[string]string) {
	t.Helper()

	content = map[string]string{}
	format = map[string]string{}

	rows, err := db.Query("SELECT Root, Format, Content FROM source")
	if err != nil {
		t.Fatalf("query source table: %v", err)
	}
	defer rows.Close()

	for rows.Next() {
		var root, f, c string
		if err := rows.Scan(&root, &f, &c); err != nil {
			t.Fatalf("scan: %v", err)
		}
		content[root] = c
		format[root] = f
	}
	return content, format
}

func decodeArchive(t *testing.T, b64 string) map[string][]byte {
	t.Helper()

	gz, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		t.Fatalf("base64 decode: %v", err)
	}
	files, err := sourcefs.ReadArchive(gz)
	if err != nil {
		t.Fatalf("read archive: %v", err)
	}
	return files
}

func TestRecordSourceArchives_RecordsAkitaFromDisk(t *testing.T) {
	db, recorder := openMemRecorder(t)

	if err := recordSourceArchives(recorder, nil); err != nil {
		t.Fatalf("recordSourceArchives: %v", err)
	}

	content, format := readSourceRows(t, db)

	b64, ok := content[sourcefs.ModulePath]
	if !ok {
		t.Fatalf("no Akita source row for %q (have %v)", sourcefs.ModulePath, keysOf(content))
	}
	if format[sourcefs.ModulePath] != sourceArchiveFormat {
		t.Errorf("format = %q, want %q", format[sourcefs.ModulePath], sourceArchiveFormat)
	}

	files := decodeArchive(t, b64)
	// Stable files that define trace vocabulary the agent reads.
	for _, want := range []string{"tracing/tracer.go", "mem/mshr/mshr.go"} {
		if _, ok := files[want]; !ok {
			t.Errorf("recorded Akita source missing %q (have %d files)", want, len(files))
		}
	}
}

func TestRecordSourceArchives_RecordsAuthorSource(t *testing.T) {
	db, recorder := openMemRecorder(t)

	authorFS := fstest.MapFS{
		"cu/compute_unit.go": {Data: []byte("package cu\n// ComputeUnit is a CU.\n")},
	}
	roots := map[string]fs.FS{"github.com/example/mysim": authorFS}

	if err := recordSourceArchives(recorder, roots); err != nil {
		t.Fatalf("recordSourceArchives: %v", err)
	}

	content, _ := readSourceRows(t, db)

	// Akita is still recorded from disk alongside the author's source.
	if _, ok := content[sourcefs.ModulePath]; !ok {
		t.Errorf("Akita source row missing when author source is provided")
	}

	b64, ok := content["github.com/example/mysim"]
	if !ok {
		t.Fatalf("no author source row")
	}
	files := decodeArchive(t, b64)
	if got := string(files["cu/compute_unit.go"]); got != "package cu\n// ComputeUnit is a CU.\n" {
		t.Errorf("author file content = %q", got)
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
