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

Requirements: **Go 1.27 or newer** (`go version` to check) and a checkout of
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

For migration between versions, see the
[Migration Guide](doc/tutorial/migration.md).

## Compatibility

Akita follows semantic versioning within a major version. For the built-in
components under `mem/` and `noc/`, the v5 promise covers how a simulator
builds, wires, and reads them:

- the `Definition` and its builder, and the `Comp` type name;
- the Spec: field names, JSON names, meaning, and presets;
- the Resources and the Ports field names;
- protocol messages and roles;
- exported functions that read a component, such as `dram.AverageReadLatency`.

It does not cover:

- the State and Middlewares of a built-in component. Their types are
  unexported, and they may change in any release; read a component through
  its exported functions instead;
- the tracing vocabulary: the task kinds, names, and tags a component emits;
- checkpoint files. Only the same build of a simulator restores a checkpoint;
- code under `examples/` and the acceptance test programs.

## Package organization

`simulation` is the runner and umbrella for the simulation infrastructure:
`simulation/{naming,hooking,queueing,timing,messaging,modeling,tracing,datarecording,sourcefs}`.
Hardware models remain under `mem` and `noc`; the live UI and trace viewer are
`monitoring` and `daisen`. See [the simulation guide](simulation/README.md)
for setup and [the migration guide](doc/tutorial/migration.md) for import and
monitor configuration changes.
