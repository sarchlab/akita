// Package dram defines detailed DRAM modeling: a cycle-accurate DRAM memory
// controller, a ticking component that serves memory requests on its Top
// port from a backing mem.Storage (Resources.Storage, required) and accepts
// control commands on its Control port. Build an instance with
// modeling.MakeBuilder[*Comp](DefaultSpec()), starting from DefaultSpec() or a preset such
// as DDR4Spec; see README.md.
package dram
