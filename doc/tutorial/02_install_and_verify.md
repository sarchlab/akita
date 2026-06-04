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
cd examples/03_first_component
go run main.go
```

Expected output:

```
tick 0 at 1000 ps
tick 1 at 2000 ps
tick 2 at 3000 ps
```

`1000 ps` is one cycle at 1 GHz. If you see those three lines, your
environment is ready. (You will not learn how the program works yet — the
next chapter dissects it.)

## Where to Next

The next chapter, **Getting Started**, introduces what a component is and
walks through the example you just ran, line by line.
