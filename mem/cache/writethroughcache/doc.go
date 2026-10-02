// Package writethroughcache provides a unified GPU cache implementation supporting
// multiple write policies (write-around, write-evict, write-through) via the
// Spec.WritePolicyType string field.
//
// The cache is a ticking component. The system builder builds it with
// Definition.Builder(): configuration is supplied as a whole through WithSpec
// (start from Definition.DefaultSpec), the engine and registration come from
// WithSimulation, shared/external wiring (the required storage, and the
// address-to-port mapper or remote ports) is injected through WithResources,
// and the Top, Bottom, and Control port instances, created with
// messaging.NewPort so the caller chooses the buffer sizes, are passed with
// WithPorts.
package writethroughcache
