# Akita

![GitHub Discussions](https://img.shields.io/github/discussions/sarchlab/akita)

[![Go Reference](https://pkg.go.dev/badge/github.com/sarchlab/akita/v5.svg)](https://pkg.go.dev/github.com/sarchlab/akita/v5)
[![Go Report Card](https://goreportcard.com/badge/github.com/sarchlab/akita/v5)](https://goreportcard.com/report/github.com/sarchlab/akita/v5)
[![Akita Test](https://github.com/sarchlab/akita/actions/workflows/akita_test.yml/badge.svg)](https://github.com/sarchlab/akita/actions/workflows/akita_test.yml)

Akita is a **discrete-event simulation framework** for computer architecture
research, written in Go. Like a game engine, a simulation engine is not a
simulator — it is the framework you use to build one. Akita is designed to be
modular and extensible so new architecture ideas can be modelled and composed
without rewriting the supporting infrastructure.

If you have used SimPy, OMNeT++, or gem5's event-driven core, Akita will feel
familiar.

- Documentation: <https://akitasim.dev/docs/akita/>
- Tutorial: <https://akitasim.dev/docs/akita/tutorial/>
- Examples: [`examples/`](examples/)

## Mental Model

An Akita simulation is, almost always, a graph of **components** connected by
**connections**:

```text
+------------+  +----------+  +------------+
| Component  |--| Connect- |--| Component  |
|     A      |  |   ion    |  |     B      |
+------------+  +----------+  +------------+
      Out                            In
```

A component is a reusable, named bundle of immutable configuration
(**Spec**), mutable runtime state (**State**), and per-cycle behaviour
(**middleware**). Components send messages out their **ports**; the
connection delivers them; the destination component reads them and reacts on
its next cycle. That is the default pattern, and most of the tutorial works
at this level.

Underneath the component layer is a smaller, more general primitive: an
**engine** that owns simulated time (`timing.VTimeInPicoSec`, picoseconds),
holds **events** in a priority queue, and dispatches each event to a named
**handler** when its time comes. Components are themselves built on this
primitive — every cycle is an event the engine fires at the component. You
can drop down to the event layer directly when you need ad-hoc behaviour
that does not fit the component shape, or when you write test scaffolding.

## What Akita Gives You

- **A deterministic engine** with serial and parallel implementations.
- **A component model** with generic Spec/State separation, so component
  configuration is separate from runtime data and both are JSON-serializable
  for checkpointing.
- **A ports and connections layer** with realistic timing, including a
  zero-latency `directconnection` for simple topologies and a `noc/` package
  for mesh, PCIe, and NVLink-style networks.
- **Memory and cache components** ready to plug in: ideal memory
  controllers, write-back caches, DRAM models, TLBs, MMUs.
- **Tracing, hooks, and a live monitor** so you can see what your simulation
  is doing.
- **Checkpoint and restore** so you can stop a long simulation and resume it
  later.

## When to Use Akita

Akita is a good fit when you want to:

- Build a cycle-level or event-level model of a new architecture idea.
- Compose many small components into a system (cores, caches, NoC, memory).
- Run experiments that vary parameters and measure outcomes.
- Reuse pieces of the simulator across projects.

It is not a fit for:

- High-level performance modelling that does not need cycle resolution
  (a spreadsheet may suffice).
- Functional emulation only (no timing required).
- Hard-real-time or production system code.

## Getting Started

Requirements: **Go 1.26 or newer** (`go version` to check) and a checkout of
this repository.

```bash
git clone https://github.com/sarchlab/akita.git
cd akita
go test ./...
```

The fastest way in is the tutorial — it builds a working memory subsystem
across a handful of runnable examples. Start with [Your First
Component](https://akitasim.dev/docs/akita/tutorial/components/what_is_a_component)
or browse [`examples/`](examples/) directly.

## V5 Development

Akita v5 is being developed alongside v4. V5 packages are being added to the
v4 codebase for now, and v4 users can safely ignore these packages since
they maintain full backward compatibility. For v5 plans and progress, see
[issue #304](https://github.com/sarchlab/akita/issues/304) and the
[v5 milestone](https://github.com/sarchlab/akita/milestone).

For migration between versions, see the
[Migration Guide](doc/core/migration.md).
