package sourcefs

import (
	"bytes"
	"database/sql"
	"encoding/base64"
	"io/fs"
	"testing"

	_ "modernc.org/sqlite"
)

func makeSourceDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	db.SetMaxOpenConns(1) // ":memory:" is per-connection
	t.Cleanup(func() { db.Close() })
	return db
}

func gzTarBase64(t *testing.T, files map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := WriteArchive(&buf, files); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

func TestOpenTraceSource(t *testing.T) {
	db := makeSourceDB(t)
	if _, err := db.Exec("CREATE TABLE source (Root, Format, Content)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	content := gzTarBase64(t, map[string][]byte{
		"a.go":   []byte("package a\n"),
		"b/c.go": []byte("package c\n"),
	})
	if _, err := db.Exec(
		"INSERT INTO source (Root, Format, Content) VALUES (?, ?, ?)",
		"example.com/m", "tar.gz;base64", content,
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	src, err := OpenTraceSource(db)
	if err != nil {
		t.Fatalf("OpenTraceSource: %v", err)
	}
	if src.IsEmpty() {
		t.Fatal("expected non-empty source")
	}
	if src.Files != 2 {
		t.Errorf("Files = %d, want 2", src.Files)
	}
	if len(src.Roots) != 1 || src.Roots[0] != "example.com/m" {
		t.Errorf("Roots = %v, want [example.com/m]", src.Roots)
	}

	data, err := fs.ReadFile(src.FS(), "example.com/m/a.go")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "package a\n" {
		t.Errorf("a.go = %q, want %q", data, "package a\n")
	}
}

func TestOpenTraceSourceToleratesEmptyRoot(t *testing.T) {
	db := makeSourceDB(t)
	if _, err := db.Exec("CREATE TABLE source (Root, Format, Content)"); err != nil {
		t.Fatalf("create table: %v", err)
	}
	content := gzTarBase64(t, map[string][]byte{"foo.go": []byte("package foo\n")})
	if _, err := db.Exec(
		"INSERT INTO source (Root, Format, Content) VALUES (?, ?, ?)",
		"", "tar.gz;base64", content, // empty root must not crash the walk
	); err != nil {
		t.Fatalf("insert: %v", err)
	}

	src, err := OpenTraceSource(db)
	if err != nil {
		t.Fatalf("OpenTraceSource: %v", err)
	}
	if src.IsEmpty() {
		t.Fatal("expected the file to load under a normalized key")
	}
	if _, err := fs.ReadFile(src.FS(), "foo.go"); err != nil {
		t.Errorf("expected foo.go readable under a normalized key: %v", err)
	}
}

func TestOpenTraceSourceNoTable(t *testing.T) {
	db := makeSourceDB(t)

	src, err := OpenTraceSource(db)
	if err != nil {
		t.Fatalf("OpenTraceSource: %v", err)
	}
	if !src.IsEmpty() {
		t.Errorf("expected empty source when no source table exists")
	}
	if src.FS() != nil {
		t.Errorf("expected nil FS when no source table exists")
	}
}
