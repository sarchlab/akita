package sourcefs

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

func TestWriteReadArchiveRoundTrip(t *testing.T) {
	in := map[string][]byte{
		"a/b.go":   []byte("package b\n"),
		"c.go":     []byte("package c\n"),
		"empty.go": {},
	}

	var buf bytes.Buffer
	if err := WriteArchive(&buf, in); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}

	out, err := ReadArchive(buf.Bytes())
	if err != nil {
		t.Fatalf("ReadArchive: %v", err)
	}

	if len(out) != len(in) {
		t.Fatalf("got %d files, want %d", len(out), len(in))
	}
	for p, want := range in {
		if !bytes.Equal(out[p], want) {
			t.Errorf("file %q: got %q, want %q", p, out[p], want)
		}
	}
}

func TestWriteArchiveDeterministic(t *testing.T) {
	in := map[string][]byte{
		"z.go": []byte("z"),
		"a.go": []byte("a"),
		"m.go": []byte("m"),
	}

	var b1, b2 bytes.Buffer
	if err := WriteArchive(&b1, in); err != nil {
		t.Fatal(err)
	}
	if err := WriteArchive(&b2, in); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(b1.Bytes(), b2.Bytes()) {
		t.Error("WriteArchive is not deterministic for identical input")
	}
}

func TestArchiveFS(t *testing.T) {
	fsys := fstest.MapFS{
		"pkg/file.go": {Data: []byte("package pkg\n")},
		"top.go":      {Data: []byte("package main\n")},
	}

	gz, err := ArchiveFS(fsys)
	if err != nil {
		t.Fatalf("ArchiveFS: %v", err)
	}

	out, err := ReadArchive(gz)
	if err != nil {
		t.Fatalf("ReadArchive: %v", err)
	}

	if got := string(out["pkg/file.go"]); got != "package pkg\n" {
		t.Errorf("pkg/file.go = %q", got)
	}
	if got := string(out["top.go"]); got != "package main\n" {
		t.Errorf("top.go = %q", got)
	}
}

func TestAkitaSourceDir(t *testing.T) {
	dir, ok := AkitaSourceDir()
	if !ok {
		t.Fatal("AkitaSourceDir not found (expected to resolve when Akita is the module under test)")
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Errorf("located dir %q has no go.mod: %v", dir, err)
	}
}

func TestArchiveDirReadsRealSourceAndFilters(t *testing.T) {
	dir, ok := AkitaSourceDir()
	if !ok {
		t.Skip("Akita source not on disk")
	}

	gz, err := ArchiveDir(dir)
	if err != nil {
		t.Fatalf("ArchiveDir: %v", err)
	}
	files, err := ReadArchive(gz)
	if err != nil {
		t.Fatalf("ReadArchive: %v", err)
	}

	if _, ok := files["tracing/tracer.go"]; !ok {
		t.Errorf("archived source missing tracing/tracer.go (have %d files)", len(files))
	}

	for p := range files {
		if strings.HasSuffix(p, "_test.go") {
			t.Errorf("archive should exclude test files, found %q", p)
		}
		if strings.HasPrefix(p, ".claude/") || strings.HasPrefix(p, ".git/") {
			t.Errorf("archive should prune %q", p)
		}
	}
}
