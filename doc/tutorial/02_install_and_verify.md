---
sidebar_position: 2
---

# Install and Verify

This page is a toolchain check. Install Go, clone the repository, run the
test suite, and run one example. If everything passes, your environment is
ready and the next chapter starts the real work.

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

## Run the smallest example

```bash
cd examples/03_random_walk
go run main.go
```

Expected output:

```
hit wall at +10 after 52 steps (53000 ps)
```

That is a tiny random-walk simulation: a single component that takes one
random step per cycle until it hits ±10 and stops. The output is
deterministic — same seed, same numbers, every run — so if you see exactly
that line, your environment is ready. (You will not learn how the program
works yet — the next chapter dissects it.)

## Where to Next

The next section, **Create a Component**, introduces what a component is
and builds the example you just ran up a few lines at a time.
