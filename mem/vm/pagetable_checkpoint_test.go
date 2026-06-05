package vm_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/mem/vm"
)

// checkpointable mirrors the structural checkpoint contract; the PageTable
// interface does not expose it, so tests type-assert to reach it.
type checkpointable interface {
	SaveCheckpoint(w io.Writer) error
	LoadCheckpoint(r io.Reader) error
}

func TestPageTableCheckpointRoundTrip(t *testing.T) {
	src := vm.NewPageTable(12)
	src.Insert(vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x4000, PageSize: 4096, Valid: true})
	src.Insert(vm.Page{PID: 1, VAddr: 0x2000, PAddr: 0x5000, PageSize: 4096, Valid: true})
	src.Insert(vm.Page{
		PID: 2, VAddr: 0x1000, PAddr: 0x9000, PageSize: 4096, Valid: true, DeviceID: 3,
	})

	var buf bytes.Buffer
	if err := src.(checkpointable).SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := vm.NewPageTable(12)
	if err := dst.(checkpointable).LoadCheckpoint(&buf); err != nil {
		t.Fatalf("LoadCheckpoint: %v", err)
	}

	// Pages come back, addressable per process.
	if p, ok := dst.Find(1, 0x1000); !ok || p.PAddr != 0x4000 {
		t.Fatalf("Find(1,0x1000) = %+v, %v", p, ok)
	}
	if p, ok := dst.Find(2, 0x1000); !ok || p.PAddr != 0x9000 || p.DeviceID != 3 {
		t.Fatalf("Find(2,0x1000) = %+v, %v", p, ok)
	}
	// ReverseLookup works on the restored list.
	if p, ok := dst.ReverseLookup(0x5000); !ok || p.VAddr != 0x2000 {
		t.Fatalf("ReverseLookup(0x5000) = %+v, %v", p, ok)
	}
}

func TestPageTableCheckpointShapeMismatch(t *testing.T) {
	src := vm.NewPageTable(12)

	var buf bytes.Buffer
	if err := src.(checkpointable).SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := vm.NewPageTable(13) // different page size
	err := dst.(checkpointable).LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "log2 page size mismatch") {
		t.Fatalf("expected shape mismatch, got %v", err)
	}
}
