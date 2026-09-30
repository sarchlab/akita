// Package modeling provides the application-level component framework
// for the Akita discrete-event simulation system.
//
// A component is defined by five structs — Spec, State, Resources, Ports,
// and Middlewares — and one of three component models, each a sub-package:
//
//   - modeling/ticking: runs on a clock, one cycle of work per tick;
//   - modeling/wakeup: no clock; runs when woken by port activity or at a
//     time it asked for;
//   - modeling/event: no clock; reacts to events that carry data.
//
// This package holds what the models share: ComponentBase, Middleware, the
// tick and wakeup schedulers, checkpoint helpers, and validation of Spec and
// State types. It also holds the older API that most components still use:
// Component[S, T, R] with a middleware pipeline, EventDrivenComponent, and
// Domain for bundling components and exposing ports at a boundary.
//
// The timing package provides the simulation kernel. The messaging package
// provides ports and connections. The modeling package provides the
// higher-level abstractions that application developers typically use to build
// components.
package modeling
