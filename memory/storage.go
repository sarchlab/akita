package memory

import "errors"

// A Storage keeps the data of the guest system.
//
// A storage is an abstraction of all different type of storage including
// registers, main memory, and hard drives.
//
// The storage implementation manages the storage in units. The unit can is
// similar to the concept of page in mmemory management. For the units that
// it not touched by Read and Write function, no memory will be allocated.
//
type Storage struct {
	unitSize uint64
	capacity uint64
	data     map[uint64][]byte
}

// NewStorage creates a storage object with the specified capacity
func NewStorage(capacity uint64) *Storage {
	storage := new(Storage)

	storage.unitSize = 4096
	storage.capacity = capacity
	storage.data = make(map[uint64][]byte)

	return storage
}

// createOrGetStorageUnit retrieves a storage unit if the unit has been created
// before. Otherwise it initilizes a storage unit in the storage object
func (s *Storage) createOrGetStorageUnit(address uint64) ([]byte, error) {
	if address > s.capacity {
		return nil, errors.New("Accessing physical address beyond the storage capacity")
	}

	baseAddr, _ := s.parseAddress(address)
	unit, ok := s.data[baseAddr]
	if !ok {
		unit = make([]byte, s.unitSize, s.unitSize)
		s.data[baseAddr] = unit
	}
	return unit, nil
}

func (s *Storage) parseAddress(addr uint64) (baseAddr, inUnitAddr uint64) {
	inUnitAddr = addr % s.unitSize
	baseAddr = addr - inUnitAddr
	return
}

func (s *Storage) Read(address uint64, len uint64) ([]byte, error) {
	currAddr := address
	lenLeft := len
	dataOffset := uint64(0)
	res := make([]byte, len)

	for currAddr < address+len {
		unit, err := s.createOrGetStorageUnit(currAddr)
		if err != nil {
			return nil, err
		}

		baseAddr, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInUnit := baseAddr + s.unitSize - currAddr
		lenToRead := uint64(0)
		if lenLeft < lenLeftInUnit {
			lenToRead = lenLeft
		} else {
			lenToRead = lenLeftInUnit
		}

		copy(res[dataOffset:dataOffset+lenToRead],
			unit[inUnitAddr:inUnitAddr+lenToRead])
		lenLeft -= lenToRead
		dataOffset += lenToRead
		currAddr += lenToRead
	}

	return res, nil
}

func (s *Storage) Write(address uint64, data []byte) error {
	currAddr := address
	dataOffset := uint64(0)

	for dataOffset < uint64(len(data)) {
		unit, err := s.createOrGetStorageUnit(currAddr)
		if err != nil {
			return err
		}

		_, inUnitAddr := s.parseAddress(currAddr)
		lenLeftInData := uint64(len(data)) - dataOffset
		lenLeftInUnit := currAddr/s.unitSize*s.unitSize + s.unitSize - currAddr
		lenToWrite := uint64(0)

		if lenLeftInData < lenLeftInUnit {
			lenToWrite = lenLeftInData
		} else {
			lenToWrite = lenLeftInUnit
		}

		copy(unit[inUnitAddr:inUnitAddr+lenToWrite],
			data[dataOffset:dataOffset+lenToWrite])
		dataOffset += lenToWrite
		currAddr += lenToWrite
	}

	return nil
}
