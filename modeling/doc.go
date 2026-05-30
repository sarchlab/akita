// Package modeling provides the application-level component framework
// for the Akita discrete-event simulation system.
//
// It builds on timing events and messaging ports to provide:
//
//   - Generic Spec+State pattern via Component[S, T]
//   - Middleware pipeline for tick-based components
//   - EventDrivenComponent[S, T] for event-driven components
//   - StateRef access to component state via the global state manager
//   - Validation helpers for Spec and State types
//
// The timing package provides the simulation kernel. The messaging package
// provides ports and connections. The modeling package provides the
// higher-level abstractions that application developers typically use to build
// components.
package modeling
