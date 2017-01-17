package memory

// A Controller defines the interface to a memory component. It defines
// a read and a write function. It also defines two functions for checking
// if the memory controller can process the read and write request
type Controller interface {
	CanRead(address uint64, len uint64) bool
	CanWrite(address uint64, len uint64) bool
	Read(address uint64, len uint64) ([]byte, error)
	Write(address uint64, data []byte) error
}
