---
slug: /tutorial
sidebar_position: 1
---

# Getting Started

This guide gets you from a fresh checkout to a running Akita simulation.

## Prerequisites

- **Go 1.26 or newer** — check with `go version`.
- **Git** — for cloning the repository.

## Install

```bash
git clone https://github.com/sarchlab/akita.git
cd akita
go test ./...
```

The test suite should pass on a clean checkout. If it does not, fix that
first — every example assumes a working build.

## Run your first simulation

The simplest runnable program is a one-component model that ticks three
times and prints the current cycle:

```bash
cd examples/03_first_component
go run main.go
```

Output:

```
tick 0 at 1000 ps
tick 1 at 2000 ps
tick 2 at 3000 ps
```

If you see that, your environment is ready.

## Where to go next

Continue with [Your First Component](./components/01_first_component.md) —
a line-by-line walk through the example you just ran. The chapters after
it expand the pattern to ports and messages, then open the event layer
underneath, then build full systems.

For the project's higher-level pitch and feature list, see the
[README](https://github.com/sarchlab/akita).
