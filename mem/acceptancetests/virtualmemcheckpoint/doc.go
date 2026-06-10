// Package virtualmemcheckpoint is a full-hierarchy checkpoint/resume oracle. It
// reuses the virtualmem acceptance-test assembly — an address translator over a
// write-through L1 + write-back L2 + ideal memory controller, with a TLB / L2TLB
// / MMU / page-table translation path — but drives it with a deterministic
// generator instead of the RNG-based MemAccessAgent, so a resumed run is
// bit-identical to an uninterrupted one. It has no non-test code beyond this
// package declaration.
package virtualmemcheckpoint
