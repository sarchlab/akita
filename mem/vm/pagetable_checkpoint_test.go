package vm_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sarchlab/akita/v5/sim"

	"github.com/sarchlab/akita/v5/mem/vm"
	"github.com/stretchr/testify/require"
)

func TestConcretePageTablePreservesCheckpointPayload(t *testing.T) {
	// This is the payload shape emitted before PageTable became concrete.
	legacy := `{"log2_page_size":12,"tables":[{"pid":1,"pages":[{"pid":1,
		"p_addr":16384,"v_addr":4096,"page_size":4096,"valid":true,
		"device_id":0,"unified":false,"is_migrating":false,"is_pinned":false}]}]}`
	pt := vm.MakePageTableBuilder().WithSimulation(sim.MakeBuilder().Build()).WithLog2PageSize(12).Build("Pages")
	require.Equal(t, "Pages", pt.Name())
	require.Equal(t, uint64(12), pt.Log2PageSize())
	require.NoError(t, pt.LoadCheckpoint(strings.NewReader(legacy)))
	page, found := pt.Find(1, 4097)
	require.True(t, found)
	require.Equal(t, uint64(16384), page.PAddr)
	var saved bytes.Buffer
	require.NoError(t, pt.SaveCheckpoint(&saved))
	require.JSONEq(t, legacy, saved.String())
	t.Log("Concrete PageTable loaded the existing payload and emitted identical JSON fields and values")
}

func TestPageTableCheckpointRoundTrip(t *testing.T) {
	src := vm.NewPageTable(12)
	src.Insert(vm.Page{PID: 1, VAddr: 0x1000, PAddr: 0x4000, PageSize: 4096, Valid: true})
	src.Insert(vm.Page{PID: 1, VAddr: 0x2000, PAddr: 0x5000, PageSize: 4096, Valid: true})
	src.Insert(vm.Page{
		PID: 2, VAddr: 0x1000, PAddr: 0x9000, PageSize: 4096, Valid: true, DeviceID: 3,
	})

	var buf bytes.Buffer
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := vm.NewPageTable(12)
	if err := dst.LoadCheckpoint(&buf); err != nil {
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
	if err := src.SaveCheckpoint(&buf); err != nil {
		t.Fatalf("SaveCheckpoint: %v", err)
	}

	dst := vm.NewPageTable(13) // different page size
	err := dst.LoadCheckpoint(&buf)
	if err == nil || !strings.Contains(err.Error(), "log2 page size mismatch") {
		t.Fatalf("expected shape mismatch, got %v", err)
	}
}
