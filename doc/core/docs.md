---
slug: /
sidebar_position: 1
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

- **Packages** — reference for the first-party component libraries, in
  dependency order:
  - **[noc](/packages/noc/)** — networks-on-chip: connections and switches
    (direct connection, mesh, PCIe, NVLink-style links).
  - **[mem](/packages/mem/)** — the memory hierarchy: caches, DRAM
    controllers, TLBs, and MMUs. Built on top of `noc`.

- **Reference** — guides that sit outside the tutorial flow:
  - **[Migration (V4 → V5)](/migration)** — the breaking changes between
    Akita V4 and V5.
  - **[Magic Guide](/magic_guide)** — shortcuts for making a simulator do
    something without modeling every detail.

- **Tools** — **[Daisen](/tools/daisen/)** visualizes recorded traces, and
  **[Akita RTM](/tools/akita-rtm/)** monitors a running simulation live.
