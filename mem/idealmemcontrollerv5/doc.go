// Package idealmemcontrollerv5 provides a Spec/State based ideal memory
// controller that uses tick-driven countdowns instead of ad-hoc events.
//
// This version aligns with the V5 guidelines:
// - Spec: immutable configuration
// - State: pure data, serializable friendly
// - Ports: explicit, named ports
// - Middlewares: ordered, stateless processors acting on component state
package idealmemcontrollerv5

