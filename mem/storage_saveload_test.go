package mem

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

func TestStorageSaveLoadRoundTrip(t *testing.T) {
	s := NewStorage(16 * KB)

	// Write some data at different addresses.
	data1 := []byte("hello, world!!")
	if err := s.Write(0, data1); err != nil {
		t.Fatalf("Write(0) error: %v", err)
	}

	data2 := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := s.Write(8*KB, data2); err != nil {
		t.Fatalf("Write(8KB) error: %v", err)
	}

	// Save
	var buf bytes.Buffer
	if err := s.Save(&buf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load into a new storage
	s2 := NewStorage(0) // capacity will be overwritten by Load
	if err := s2.Load(&buf); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Verify capacity
	if s2.Capacity != 16*KB {
		t.Errorf("Capacity = %d, want %d", s2.Capacity, 16*KB)
	}

	// Verify data at address 0
	got1, err := s2.Read(0, uint64(len(data1)))
	if err != nil {
		t.Fatalf("Read(0) error: %v", err)
	}

	if !bytes.Equal(got1, data1) {
		t.Errorf("data at 0 = %v, want %v", got1, data1)
	}

	// Verify data at address 8KB
	got2, err := s2.Read(8*KB, uint64(len(data2)))
	if err != nil {
		t.Fatalf("Read(8KB) error: %v", err)
	}

	if !bytes.Equal(got2, data2) {
		t.Errorf("data at 8KB = %v, want %v", got2, data2)
	}
}

func TestStorageSaveLoadEmptyStorage(t *testing.T) {
	s := NewStorage(4 * KB)

	var buf bytes.Buffer
	if err := s.Save(&buf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	s2 := NewStorage(0)
	if err := s2.Load(&buf); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if s2.Capacity != 4*KB {
		t.Errorf("Capacity = %d, want %d", s2.Capacity, 4*KB)
	}

	if len(s2.data) != 0 {
		t.Errorf("data map len = %d, want 0", len(s2.data))
	}
}

func TestStorageSaveLoadCustomUnitSize(t *testing.T) {
	s := NewStorageWithUnitSize(8*KB, 512)

	data := bytes.Repeat([]byte{0xAB}, 1024)
	if err := s.Write(0, data); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var buf bytes.Buffer
	if err := s.Save(&buf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	s2 := NewStorage(0)
	if err := s2.Load(&buf); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if s2.unitSize != 512 {
		t.Errorf("unitSize = %d, want 512", s2.unitSize)
	}

	got, err := s2.Read(0, 1024)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	if !bytes.Equal(got, data) {
		t.Error("data mismatch after round-trip with custom unit size")
	}
}

func readBinaryHeader(
	t *testing.T, r io.Reader,
) (capacity, unitSize, count uint64) {
	t.Helper()

	if err := binary.Read(r, binary.LittleEndian, &capacity); err != nil {
		t.Fatalf("read capacity: %v", err)
	}

	if err := binary.Read(r, binary.LittleEndian, &unitSize); err != nil {
		t.Fatalf("read unitSize: %v", err)
	}

	if err := binary.Read(r, binary.LittleEndian, &count); err != nil {
		t.Fatalf("read count: %v", err)
	}

	return capacity, unitSize, count
}

func TestStorageSaveBinaryFormat(t *testing.T) {
	s := NewStorageWithUnitSize(1*KB, 64)

	data := make([]byte, 64)
	for i := range data {
		data[i] = byte(i)
	}

	if err := s.Write(0, data); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var buf bytes.Buffer
	if err := s.Save(&buf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	r := bytes.NewReader(buf.Bytes())

	capacity, unitSize, count := readBinaryHeader(t, r)

	if capacity != 1*KB {
		t.Errorf("capacity = %d, want %d", capacity, 1*KB)
	}

	if unitSize != 64 {
		t.Errorf("unitSize = %d, want 64", unitSize)
	}

	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}

	// Read the single entry
	var baseAddr uint64
	if err := binary.Read(r, binary.LittleEndian, &baseAddr); err != nil {
		t.Fatalf("read baseAddr: %v", err)
	}

	if baseAddr != 0 {
		t.Errorf("baseAddr = %d, want 0", baseAddr)
	}

	unitData := make([]byte, 64)
	if _, err := io.ReadFull(r, unitData); err != nil {
		t.Fatalf("read unit data: %v", err)
	}

	if !bytes.Equal(unitData, data) {
		t.Error("unit data mismatch")
	}
}

func TestStorageLoadInvalidData(t *testing.T) {
	s := NewStorage(4 * KB)

	// Too short to contain even the header
	r := bytes.NewReader([]byte{0x01, 0x02})

	err := s.Load(r)
	if err == nil {
		t.Error("Load() expected error for truncated data, got nil")
	}
}

func TestStorageLoadReplacesExistingData(t *testing.T) {
	// Create storage with some data
	s := NewStorage(4 * KB)
	if err := s.Write(0, []byte("old data")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Create a different storage and save it
	s2 := NewStorageWithUnitSize(8*KB, 256)
	if err := s2.Write(256, []byte("new data")); err != nil {
		t.Fatalf("Write error: %v", err)
	}

	var buf bytes.Buffer
	if err := s2.Save(&buf); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Load into the original storage
	if err := s.Load(&buf); err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	// Old data should be gone
	if s.Capacity != 8*KB {
		t.Errorf("Capacity = %d, want %d", s.Capacity, 8*KB)
	}

	got, err := s.Read(256, 8)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}

	if string(got) != "new data" {
		t.Errorf("data = %q, want %q", string(got), "new data")
	}
}

// errWriterMem always returns an error on Write.
type errWriterMem struct{}

func (e *errWriterMem) Write([]byte) (int, error) {
	return 0, io.ErrClosedPipe
}

func TestStorageSaveWriteError(t *testing.T) {
	s := NewStorage(4 * KB)

	err := s.Save(&errWriterMem{})
	if err == nil {
		t.Error("Save() expected error for writer failure, got nil")
	}
}
