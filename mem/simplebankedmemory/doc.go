// Package simplebankedmemory provides a configurable banked memory component.
//
// When several instances are used as interleaved controllers over one shared,
// globally-addressed storage, enable the bank-selection address conversion
// (Spec.BankAddrConvEnabled) so bank selection operates on a controller-local
// address rather than the strided global address; otherwise accesses alias
// onto a fraction of the banks. See the BankAddrConv* fields on Spec.
package simplebankedmemory
