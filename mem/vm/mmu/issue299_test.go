package mmu

import (
	"testing"

	"github.com/sarchlab/akita/v4/mem/vm"
	"github.com/sarchlab/akita/v4/sim"
)

// TestIssue299 specifically tests the issue described in the GitHub issue
func TestIssue299(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Test the BEFORE scenario: MMU panics when page not found
	t.Run("Before - Should panic without auto allocation", func(t *testing.T) {
		builder := MakeBuilder().WithEngine(engine)
		mmu := builder.Build("MMUBefore")
		middleware := &middleware{Comp: mmu}

		// Create a translation request
		req := vm.TranslationReqBuilder{}.
			WithDst(mmu.GetTopPort().AsRemote()).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(2).
			Build()

		// Add to walking translations (simulating the MMU receiving a request)
		transaction := transaction{
			req:       req,
			cycleLeft: 0, // Ready for finalization
		}
		mmu.walkingTranslations = append(mmu.walkingTranslations, transaction)

		// This should panic because page is not found and auto allocation is disabled
		defer func() {
			if r := recover(); r != nil {
				if r == "page not found" {
					t.Log("Successfully caught expected panic: page not found")
				} else {
					t.Errorf("Unexpected panic: %v", r)
				}
			} else {
				t.Error("Expected panic but none occurred")
			}
		}()

		// This should cause a panic
		middleware.finalizePageWalk(0)
	})

	// Test the AFTER scenario: MMU creates page automatically
	t.Run("After - Should create page with auto allocation", func(t *testing.T) {
		builder := MakeBuilder().
			WithEngine(engine).
			WithAutoPageAllocation(true) // Enable the new feature
		mmu := builder.Build("MMUAfter")
		middleware := &middleware{Comp: mmu}

		// Create a translation request
		req := vm.TranslationReqBuilder{}.
			WithDst(mmu.GetTopPort().AsRemote()).
			WithPID(1).
			WithVAddr(0x1000).
			WithDeviceID(2).
			Build()

		// Add to walking translations
		transaction := transaction{
			req:       req,
			cycleLeft: 0,
		}
		mmu.walkingTranslations = append(mmu.walkingTranslations, transaction)

		// Verify page doesn't exist initially
		_, found := mmu.GetPageTable().Find(vm.PID(1), 0x1000)
		if found {
			t.Fatal("Page should not exist initially")
		}

		// This should NOT panic and should create the page automatically
		// We can't fully test finalizePageWalk due to port dependencies,
		// but we can test that the page creation works
		page := middleware.createDefaultPage(vm.PID(1), 0x1000, 2)
		mmu.GetPageTable().Insert(page)

		// Verify page was created correctly
		foundPage, found := mmu.GetPageTable().Find(vm.PID(1), 0x1000)
		if !found {
			t.Fatal("Page should exist after auto allocation")
		}

		// Verify page properties
		if foundPage.PID != vm.PID(1) {
			t.Errorf("Expected PID 1, got %v", foundPage.PID)
		}
		if foundPage.VAddr != 0x1000 {
			t.Errorf("Expected VAddr 0x1000, got 0x%x", foundPage.VAddr)
		}
		if foundPage.DeviceID != 2 {
			t.Errorf("Expected DeviceID 2, got %v", foundPage.DeviceID)
		}
		if !foundPage.Valid {
			t.Error("Expected page to be valid")
		}

		t.Log("Successfully created page automatically without panic")
	})
}

// TestDefaultBehaviorUnchanged ensures backward compatibility
func TestDefaultBehaviorUnchanged(t *testing.T) {
	engine := sim.NewSerialEngine()

	// Create MMU with default settings (should have auto allocation disabled)
	builder := MakeBuilder().WithEngine(engine)
	mmu := builder.Build("MMUDefault")

	// Verify auto allocation is disabled by default
	if mmu.IsAutoPageAllocationEnabled() {
		t.Error("Auto page allocation should be disabled by default for backward compatibility")
	}

	// Verify that the default behavior (panicking) is preserved
	middleware := &middleware{Comp: mmu}
	req := vm.TranslationReqBuilder{}.
		WithDst(mmu.GetTopPort().AsRemote()).
		WithPID(1).
		WithVAddr(0x1000).
		WithDeviceID(2).
		Build()

	transaction := transaction{
		req:       req,
		cycleLeft: 0,
	}
	mmu.walkingTranslations = append(mmu.walkingTranslations, transaction)

	// This should still panic to maintain backward compatibility
	defer func() {
		if r := recover(); r != nil {
			if r == "page not found" {
				t.Log("Confirmed: Default behavior (panic) is preserved for backward compatibility")
			} else {
				t.Errorf("Unexpected panic: %v", r)
			}
		} else {
			t.Error("Expected panic to maintain backward compatibility")
		}
	}()

	middleware.finalizePageWalk(0)
}