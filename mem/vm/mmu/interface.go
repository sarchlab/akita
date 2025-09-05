package mmu

// pageSizeGetter is an optional interface that page tables can implement
// to expose their page size for validation purposes.
type pageSizeGetter interface {
	GetLog2PageSize() uint64
}