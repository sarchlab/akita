package mem

import (
	"bytes"
	"math"
	"testing"
)

func TestStorageRejectsInvalidRanges(t *testing.T) {
	tests := []struct {
		name     string
		capacity uint64
		address  uint64
		length   uint64
	}{
		{"empty at start", 10, 0, 0},
		{"empty at capacity", 10, 10, 0},
		{"empty beyond capacity", 10, 11, 0},
		{"empty at maximum address", 10, math.MaxUint64, 0},
		{"zero capacity", 0, 0, 1},
		{"empty zero capacity", 0, 0, 0},
		{"start at capacity", 10, 10, 1},
		{"start beyond capacity", 10, 11, 1},
		{"cross capacity within unit", 10, 9, 2},
		{"cross capacity across units", 10, 2, 12},
		{"start at aligned capacity", 12, 12, 1},
		{"cross aligned capacity", 12, 2, 12},
		{"overflow end address", math.MaxUint64, math.MaxUint64 - 1, 4},
		{"maximum address", math.MaxUint64, math.MaxUint64, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, operation := range []string{"read", "write"} {
				t.Run(operation, func(t *testing.T) {
					testInvalidStorageRange(t, tt.capacity, tt.address, tt.length, operation)
				})
			}
		})
	}
}

func testInvalidStorageRange(t *testing.T, capacity, address, length uint64, operation string) {
	t.Helper()

	s := NewStorageWithUnitSize(capacity, 4)
	original := []byte{1, 2, 3, 4}
	if capacity >= 4 {
		s.Write(0, original)
	}
	unitsBefore := len(s.data)
	mustPanicStorage(t, func() {
		if operation == "read" {
			s.Read(address, length)
		} else {
			s.Write(address, make([]byte, length))
		}
	})
	if len(s.data) != unitsBefore {
		t.Errorf("invalid range allocated units: %d -> %d", unitsBefore, len(s.data))
	}
	if unitsBefore > 0 && !bytes.Equal(s.data[0].data, original) {
		t.Errorf("invalid range changed stored bytes: %v", s.data[0].data)
	}
}

func TestStorageRejectsHugeReadBeforeAllocation(t *testing.T) {
	s := NewStorageWithUnitSize(10, 4)
	mustPanicStorage(t, func() { s.Read(1, math.MaxUint64) })
	if len(s.data) != 0 {
		t.Fatal("rejected read allocated storage units")
	}
}

func TestStorageValidBoundaryRanges(t *testing.T) {
	tests := []struct {
		name     string
		capacity uint64
		unitSize uint64
		address  uint64
		data     []byte
	}{
		{"last byte", 10, 4, 9, []byte{1}},
		{"end in partial unit", 10, 4, 6, []byte{1, 2, 3, 4}},
		{"end at unit boundary", 12, 4, 8, []byte{1, 2, 3, 4}},
		{"cross multiple units", 10, 4, 1, []byte{1, 2, 3, 4, 5, 6, 7, 8, 9}},
		{"unit larger than capacity", 3, 8, 0, []byte{1, 2, 3}},
		{"near maximum capacity", math.MaxUint64, 4, math.MaxUint64 - 4, []byte{1, 2, 3, 4}},
		{"non-power-of-two unit", math.MaxUint64, 7, math.MaxUint64 - 4, []byte{1, 2, 3, 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := NewStorageWithUnitSize(tt.capacity, tt.unitSize)
			data := s.Read(tt.address, uint64(len(tt.data)))
			if !bytes.Equal(data, make([]byte, len(tt.data))) {
				t.Fatalf("untouched boundary read = %v", data)
			}
			s.Write(tt.address, tt.data)
			data = s.Read(tt.address, uint64(len(tt.data)))
			if !bytes.Equal(data, tt.data) {
				t.Fatalf("boundary round trip = %v, want %v", data, tt.data)
			}
		})
	}
}

func mustPanicStorage(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Error("invalid storage range did not panic")
		}
	}()
	fn()
}
