// Package writethrough provides a unified GPU cache implementation supporting
// multiple write policies (write-around, write-evict, write-through) via the
// Spec.WritePolicyType string field.
//
// The cache is a ticking component. The system builder builds it with
// Definition.Builder(): configuration is supplied as a whole through WithSpec
// (start from Definition.DefaultSpec), the engine and registration come from
// WithSimulation, the required shared storage is supplied through WithResources,
// and routing is configured by Spec.Bottom.
// and the Top, Bottom, and Control port instances, created with
// twowaybuffered.NewPort so the caller chooses the buffer sizes, are passed with
// BindPort after construction.
package writethrough
