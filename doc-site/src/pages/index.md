---
title: Akita
hide_table_of_contents: true
---

# Akita

Akita is a **discrete-event simulation framework for computer architecture
research**, written in Go. It gives you a generic component model, a
deterministic engine, ready-made memory and network components, and tools to
observe and record what a simulation does.

## Where to start

- **[Tutorial](/tutorial/)** — the place to begin. It builds up from a single
  component to components that talk to each other, then hooks and tracing for
  observing a run, and finally the raw event layer underneath. Every chapter
  has a runnable example.

- **Core** — reference for the framework packages, in dependency order:
  [hooking](/packages/hooking/), [naming](/packages/naming/),
  [timing](/packages/timing/), [queueing](/packages/queueing/),
  [datarecording](/packages/datarecording/), [messaging](/packages/messaging/),
  [modeling](/packages/modeling/), [tracing](/packages/tracing/),
  [simulation](/packages/simulation/), and [examples](/packages/examples/).

- **First-party components** — reference for the ready-made component
  libraries:
  - **[noc](/packages/noc/)** — networks-on-chip: connections and switches
    (direct connection, mesh, PCIe, NVLink-style links).
  - **[mem](/packages/mem/)** — the memory hierarchy: caches, DRAM
    controllers, TLBs, and MMUs. Built on top of `noc`.

- **Reference** — guides that sit outside the tutorial flow (under the Tutorial
  tab):
  - **[Migration (V4 → V5)](/tutorial/migration)** — the breaking changes
    between Akita V4 and V5.
  - **[Magic Guide](/tutorial/magic_guide)** — shortcuts for making a simulator
    do something without modeling every detail.
  - **[Writing Checkpointable Code](/tutorial/checkpointing)** — how to make
    messages, events, and components survive checkpoint/resume.

- **Tools** — **[Daisen](/tools/daisen/)** visualizes recorded traces, and
  **[Akita RTM](/tools/akita-rtm/)** monitors a running simulation live.
