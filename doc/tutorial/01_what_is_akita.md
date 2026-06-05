---
slug: /tutorial
sidebar_position: 1
---

# What is Akita?

Akita is a **discrete-event simulation framework** for computer architecture
research, written in Go. It is not itself a simulator — it is the engine you
use to build simulators.

If you have used SimPy, OMNeT++, or gem5's event-driven core, Akita will
feel familiar. If you have not, the rest of this page explains the mental
model and shows the shape of what you will build in this tutorial.

## The Mental Model

An Akita simulation is, almost always, a graph of **components** connected
by **connections**:

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
connection delivers them; the destination component reads them and reacts
on its next cycle. That is the default pattern, and most of this tutorial
works at this level.

Underneath the component layer is a smaller, more general primitive: an
**engine** that owns simulated time (`timing.VTimeInSec`, picoseconds),
holds **events** in a priority queue, and dispatches each event to a
named **handler** when its time comes. Components are themselves built
on this primitive — every cycle is an event the engine fires at the
component. You can drop down to the event layer directly when you need
ad-hoc behaviour that does not fit the component shape, or when you write
test scaffolding.

## What Akita Gives You

- **A deterministic engine** with serial and parallel implementations.
- **A component model** with generic Spec/State separation, so component
  configuration is separate from runtime data and both are
  JSON-serializable for checkpointing.
- **A ports and connections layer** with realistic timing, including a
  zero-latency `directconnection` for simple topologies and a `noc/`
  package for mesh, PCIe, and NVLink-style networks.
- **Memory and cache components** ready to plug in: ideal memory
  controllers, write-back caches, DRAM models, TLBs, MMUs.
- **Tracing, hooks, and a live monitor** so you can see what your
  simulation is doing.
- **Checkpoint and restore** so you can stop a long simulation and resume
  it later.

## When to Use Akita

Akita is a good fit when you want to:

- Build a cycle-level or event-level model of a new architecture idea.
- Compose many small components into a system (cores, caches, NoC,
  memory).
- Run experiments that vary parameters and measure outcomes.
- Reuse pieces of the simulator across projects.

It is not a fit for:

- High-level performance modelling that does not need cycle resolution
  (a spreadsheet may suffice).
- Functional emulation only (no timing required).
- Hard-real-time or production system code.

## What You Will Build in This Tutorial

Each chapter is one runnable example. You can read the code, run it, and
modify it. The chapters build on each other:

1. **Create a component** — the default Akita pattern. Write a component
   with Spec, State, and middleware, built up a few lines at a time.
2. **Make components talk to each other** — add ports and messages, and
   connect two components so they can communicate.
3. **Getting information from a simulation** — observe a running simulation
   with hooks that log events and messages, and measure work with tracing
   tasks.
4. **Event-based simulation** — open the layer underneath: schedule
   events directly, write custom event types, and use event-driven
   components for the idle case.

By the end you will be able to write components, connect them into a
simulation, observe what they do, and drop to the event layer when the
component pattern does not fit.

## Where to Next

The next chapter is a short **Install and Verify** — install Go, clone
the repository, and run the smallest example to confirm your environment
works. After that, **Create a Component** introduces what a component is
and builds that example up a few lines at a time.
