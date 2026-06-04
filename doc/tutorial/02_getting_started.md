---
sidebar_position: 2
---

# Getting Started

This page gets you from a fresh checkout to a running Akita simulation.
You will not learn how the simulation works here — that is the job of the
chapters that follow. The point of this page is to confirm your toolchain
is set up correctly.

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
before continuing — every example assumes a working build.

## Run your first simulation

The smallest runnable Akita program is a one-component model that ticks
three times and prints the current cycle:

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

`1000 ps` is one cycle at 1 GHz. If you see those three lines, your
environment is ready.

## Where to Next

The next chapter, **Your First Component**, walks through that example
line by line — what a component is, what runs every cycle, and how the
engine decides when to stop.
