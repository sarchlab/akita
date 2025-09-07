package idealmemcontrollerv5

// Local abstraction layer for external dependencies. The goal is for the
// idealmemcontrollerv5 package to depend on these interfaces only, so tests
// can mock them without importing external packages.
//
//go:generate mockgen -destination "mock_local_test.go" -package $GOPACKAGE -write_package_comment=false -source interface.go

// Storage models the minimal storage operations used by the controller.
// It is implemented by mem.Storage.
type Storage interface {
    Read(address uint64, length uint64) ([]byte, error)
    Write(address uint64, data []byte) error
}

// AddressConverter models address translation strategy.
// It is implemented by types like mem.InterleavingConverter.
type AddressConverter interface {
    ConvertExternalToInternal(external uint64) uint64
    ConvertInternalToExternal(internal uint64) uint64
}

// StateAccessor provides read access to shared state by ID.
// It is implemented by simv5.Simulation.
type StateAccessor interface {
    GetState(id string) (interface{}, bool)
}
