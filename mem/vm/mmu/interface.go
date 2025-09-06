package mmu

import "github.com/sarchlab/akita/v4/mem/vm"

// pageTable aggregates all the methods of the page table that are used in the MMU package.
type pageTable interface {
	Insert(page vm.Page)
	Remove(pid vm.PID, vAddr uint64)
	Find(pid vm.PID, Addr uint64) (vm.Page, bool)
	Update(page vm.Page)
	ReverseLookup(pAddr uint64) (vm.Page, bool)
	GetLog2PageSize() uint64
}