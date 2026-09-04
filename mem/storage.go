package mem

import (
	"errors"
	"sync"
)

// For capacity
const (
	_         = iota
	KB uint64 = 1 << (10 * iota)
	MB
	GB
	TB
)

// A Storage keeps the data of the guest system.
//
// A storage is an abstraction of all different type of storage including
// registers, main memory, and hard drives.
//
// The storage implementation manages the storage in units. The unit can is
// similar to the concept of page in memory management. For the units that
// it not touched by Read and Write function, no memory will be allocated.
type Storage struct {
	sync.Mutex

	name     string
	capacity uint64
	unitSize uint64
	data     map[uint64]*storageUnit
}

// Name returns the name of the storage. It is empty for storage created through
// the bare NewStorage constructors; use StorageBuilder to give it a name and
// register it as a simulation resource.
func (s *Storage) Name() string {
	return s.name
}

// Capacity returns the capacity of the storage in bytes.
func (s *Storage) Capacity() uint64 {
	return s.capacity
}

type storageUnit struct {
	sync.RWMutex

	data []byte
}

func newStorageUnit(uintSize uint64) *storageUnit {
	u := new(storageUnit)
	u.data = make([]byte, uintSize)

	return u
}

// NewStorage creates an unnamed, unregistered storage with the specified
// capacity. Prefer StorageBuilder for storage that should be a named simulation
// resource.
func NewStorage(capacity uint64) *Storage {
	return MakeStorageBuilder().WithCapacity(capacity).Build("")
}

// NewStorageWithUnitSize creates an unnamed, unregistered storage with the
// specified capacity and unit size (in bytes). Using a smaller unit size
// reduces the memory consumption of storage. Prefer StorageBuilder for storage
// that should be a named simulation resource.
func NewStorageWithUnitSize(capacity uint64, unitSize uint64) *Storage {
	return MakeStorageBuilder().
		WithCapacity(capacity).
		WithUnitSize(unitSize).
		Build("")
}

// createOrGetStorageUnit retrieves a storage unit if the unit has been created
// before. Otherwise it initializes a storage unit in the storage object.
// The caller must validate the access range first.
func (s *Storage) createOrGetStorageUnit(address uint64) *storageUnit {
	baseAddr, _ := s.parseAddress(address)

	s.Lock()
	defer s.Unlock()

	unit, ok := s.data[baseAddr]
	if !ok {
		unit = newStorageUnit(s.unitSize)
		s.data[baseAddr] = unit
	}

	return unit
}

func (s *Storage) parseAddress(addr uint64) (baseAddr, inUnitAddr uint64) {
	inUnitAddr = addr % s.unitSize
	baseAddr = addr - inUnitAddr

	return
}

func (s *Storage) validateRange(address, length uint64) error {
	if length == 0 {
		return errors.New("storage access length must be greater than zero")
	}
	// Subtraction avoids overflowing address + length.
	if address > s.capacity || length > s.capacity-address {
		return errors.New("accessing physical address beyond the storage capacity")
	}

	return nil
}

// Read returns length bytes starting at address. It returns an error for an
// empty range or a range extending beyond capacity, before allocating data or
// storage units. A nonempty range may end exactly at capacity.
func (s *Storage) Read(address uint64, length uint64) ([]byte, error) {
	if err := s.validateRange(address, length); err != nil {
		return nil, err
	}

	currAddr := address
	lenLeft := length
	dataOffset := uint64(0)
	res := make([]byte, length)

	for lenLeft > 0 {
		unit := s.createOrGetStorageUnit(currAddr)

		_, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInUnit := s.unitSize - inUnitAddr

		var lenToRead uint64
		if lenLeft < lenLeftInUnit {
			lenToRead = lenLeft
		} else {
			lenToRead = lenLeftInUnit
		}

		copy(res[dataOffset:dataOffset+lenToRead],
			unit.data[inUnitAddr:inUnitAddr+lenToRead])

		lenLeft -= lenToRead
		dataOffset += lenToRead
		currAddr += lenToRead
	}

	return res, nil
}

// Write stores data starting at address. It returns an error for empty data or
// a range extending beyond capacity, without allocating storage units or
// changing stored bytes. A nonempty range may end exactly at capacity.
func (s *Storage) Write(address uint64, data []byte) error {
	if err := s.validateRange(address, uint64(len(data))); err != nil {
		return err
	}

	currAddr := address
	dataOffset := uint64(0)

	for dataOffset < uint64(len(data)) {
		unit := s.createOrGetStorageUnit(currAddr)

		_, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInData := uint64(len(data)) - dataOffset
		lenLeftInUnit := s.unitSize - inUnitAddr

		var lenToWrite uint64
		if lenLeftInData < lenLeftInUnit {
			lenToWrite = lenLeftInData
		} else {
			lenToWrite = lenLeftInUnit
		}

		copy(unit.data[inUnitAddr:inUnitAddr+lenToWrite],
			data[dataOffset:dataOffset+lenToWrite])

		dataOffset += lenToWrite
		currAddr += lenToWrite
	}

	return nil
}
