package checkpoint

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEntityPathEscapesNames(t *testing.T) {
	cases := map[string]string{
		"Engine":                        "entities/Engine",
		"GPU[1].SA[0].CU[2].MemoryPort": "entities/GPU%5B1%5D.SA%5B0%5D.CU%5B2%5D.MemoryPort",
		"Program/Memory With Spaces":    "entities/Program%2FMemory%20With%20Spaces",
	}

	for name, want := range cases {
		if got := EntityPath(name); got != want {
			t.Fatalf("EntityPath(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestWriteReadArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.tar.gz")

	entries := []ArchiveEntry{
		{Name: "GPU[0].State", Data: []byte(`{"ok":true}`)},
		{Name: "Engine", Data: []byte("binary-bytes")},
	}

	if err := WriteArchive(path, "build-1", entries); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temporary archive was not removed")
	}

	buildID, payloads, err := ReadArchive(path)
	if err != nil {
		t.Fatalf("ReadArchive: %v", err)
	}
	if buildID != "build-1" {
		t.Fatalf("build ID = %q, want build-1", buildID)
	}

	want := map[string][]byte{
		"GPU[0].State": []byte(`{"ok":true}`),
		"Engine":       []byte("binary-bytes"),
	}
	if !reflect.DeepEqual(payloads, want) {
		t.Fatalf("payloads = %v, want %v", payloads, want)
	}
}

func TestWriteArchiveSortsEntriesBuildIDFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "checkpoint.tar.gz")

	entries := []ArchiveEntry{
		{Name: "b", Data: []byte("b")},
		{Name: "a", Data: []byte("a")},
	}

	if err := WriteArchive(path, "build-1", entries); err != nil {
		t.Fatalf("WriteArchive: %v", err)
	}

	names := archiveEntryNames(t, path)
	want := []string{buildIDPath, EntityPath("a"), EntityPath("b")}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("archive names = %v, want %v", names, want)
	}
}

func TestWriteArchiveRejectsEmptyBuildID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	if err := WriteArchive(path, "", nil); err == nil {
		t.Fatalf("expected error for empty build ID")
	}
}

func TestWriteArchiveRejectsDuplicateEntity(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.tar.gz")
	entries := []ArchiveEntry{
		{Name: "dup", Data: []byte("1")},
		{Name: "dup", Data: []byte("2")},
	}
	if err := WriteArchive(path, "build-1", entries); err == nil ||
		!strings.Contains(err.Error(), "duplicate entity") {
		t.Fatalf("expected duplicate-entity error, got %v", err)
	}
}

func archiveEntryNames(t *testing.T, path string) []string {
	t.Helper()

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("os.Open: %v", err)
	}
	defer file.Close()

	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatalf("gzip.NewReader: %v", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	names := []string{}
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar.Next: %v", err)
		}

		names = append(names, header.Name)
	}

	return names
}
