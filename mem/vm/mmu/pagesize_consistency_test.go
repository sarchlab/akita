package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
)

func TestPageSizeConsistencyCheck(t *testing.T) {
	t.Run("should not panic when page table page size matches MMU page size", func(t *testing.T) {
		pageTable := vm.NewPageTable(12) // 4KB pages (2^12)
		
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Expected no panic, but got: %v", r)
			}
		}()
		
		// This should not panic
		builder := MakeBuilder().
			WithLog2PageSize(12).
			WithPageTable(pageTable)
		
		builder.validatePageTablePageSize()
	})
	
	t.Run("should panic when page table page size does not match MMU page size", func(t *testing.T) {
		pageTable := vm.NewPageTable(12) // 4KB pages (2^12)
		
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic but got none")
			} else if r != "page table page size does not match MMU page size" {
				t.Errorf("Expected specific panic message, got: %v", r)
			}
		}()
		
		// This should panic
		builder := MakeBuilder().
			WithLog2PageSize(16). // 64KB pages (2^16) - different from page table
			WithPageTable(pageTable)
		
		builder.validatePageTablePageSize()
	})
	
	t.Run("should not panic when no page table is provided", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Expected no panic, but got: %v", r)
			}
		}()
		
		// This should not panic (pageTable is nil)
		builder := MakeBuilder().WithLog2PageSize(12)
		builder.validatePageTablePageSize()
	})
}

func TestPageTableImplementsPageSizeGetter(t *testing.T) {
	pageTable := vm.NewPageTable(12)
	
	// Check that the default page table implements PageSizeGetter
	if pageSizeGetter, ok := pageTable.(PageSizeGetter); !ok {
		t.Error("Default page table should implement PageSizeGetter interface")
	} else {
		if pageSizeGetter.GetLog2PageSize() != 12 {
			t.Errorf("Expected page size 12, got %d", pageSizeGetter.GetLog2PageSize())
		}
	}
}

func TestBuilderValidatesPageTablePageSize(t *testing.T) {
	t.Run("should panic during build when page sizes don't match", func(t *testing.T) {
		pageTable := vm.NewPageTable(12) // 4KB pages
		
		defer func() {
			if r := recover(); r == nil {
				t.Error("Expected panic but got none")
			} else if r != "page table page size does not match MMU page size" {
				t.Errorf("Expected specific panic message, got: %v", r)
			}
		}()
		
		// This should panic during Build()
		MakeBuilder().
			WithPageTable(pageTable).
			WithLog2PageSize(16). // Different page size
			Build("TestMMU")
	})
	
	t.Run("should not panic during build when page sizes match", func(t *testing.T) {
		pageTable := vm.NewPageTable(12) // 4KB pages
		
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("Expected no panic, but got: %v", r)
			}
		}()
		
		// This should not panic
		mmu := MakeBuilder().
			WithPageTable(pageTable).
			WithLog2PageSize(12). // Same page size
			Build("TestMMU")
		
		if mmu == nil {
			t.Error("Expected MMU to be created")
		}
	})
}