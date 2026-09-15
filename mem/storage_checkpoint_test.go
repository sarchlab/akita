package mem_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem"
)

func TestStorageCheckpointRoundTrip(t *testing.T) {
	src := mem.NewStorage(1 * mem.MB)
	src.Write(0, []byte{1, 2, 3, 4})
	// A second write in a different allocation unit (default unit size 4 KB).
	src.Write(64*mem.KB, []byte{9, 9, 9})

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := mem.NewStorage(1 * mem.MB)
	if err := dst.LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	got := dst.Read(0, 4)
	if !bytes.Equal(got, []byte{1, 2, 3, 4}) {
		t.Fatalf("Read(0,4) = %v, want [1 2 3 4]", got)
	}
	got = dst.Read(64*mem.KB, 3)
	if !bytes.Equal(got, []byte{9, 9, 9}) {
		t.Fatalf("Read(64KB,3) = %v, want [9 9 9]", got)
	}
	// An untouched region restores as zeros.
	got = dst.Read(4, 4)
	if !bytes.Equal(got, []byte{0, 0, 0, 0}) {
		t.Fatalf("untouched Read = %v, want zeros", got)
	}
}

func TestStorageCheckpointCapacityMismatch(t *testing.T) {
	src := mem.NewStorage(1 * mem.MB)

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := mem.NewStorage(2 * mem.MB)
	err := dst.LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "capacity mismatch") {
		t.Fatalf("expected capacity mismatch, got %v", err)
	}
}
