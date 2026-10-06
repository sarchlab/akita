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
// This package holds what the models share: the Component and Middleware
// interfaces, checkpoint helpers, validation of Spec and State types, and
// Domain for bundling components and exposing ports at a boundary. Each model
// package holds its own scheduler and the event it schedules. The base every
// model's Component embeds, and the steps of Build, are in modeling/internal.
//
// The timing package provides the simulation kernel. The messaging package
// provides ports and connections. The modeling package provides the
// higher-level abstractions that application developers typically use to build
// components.
package modeling
