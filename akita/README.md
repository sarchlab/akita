# akita — Command-Line Tool

`akita` is the command-line tool for common tasks related to developing
simulators with the Akita framework. It is a [cobra](https://github.com/spf13/cobra)
CLI that scaffolds new components and lints existing component packages so they
follow Akita conventions.

## Install

```sh
go install github.com/sarchlab/akita/v5/akita@latest
```

This produces an `akita` binary on your `PATH`. Run `akita --help` to list the
available subcommands.

## Subcommands

### `akita component --create <Name>`

Scaffolds a new component package. The command must be run inside a Git
repository. It creates a folder named `<Name>` and, from embedded templates,
writes two files:

- `comp.go` — a `Comp` struct embedding `sim.TickingComponent` and
  `sim.MiddlewareHolder`, with a `Tick()` method.
- `builder.go` — a `Builder` struct (with `engine` and `freq` fields), plus
  `MakeBuilder`, `WithEngine`, `WithFreq`, and `Build` methods.

The package name in both files is set to `<Name>`.

```sh
akita component --create MyCache
# Component 'MyCache' created successfully!
# Builder file generated successfully!
# Comp file generated successfully!
```

The command fails if the folder already exists or if it is not run inside a Git
working tree.

### `akita check <component folder path>`

Lints a component package (the akitav5-lint check) and exits non-zero if any
rule is violated. It validates three areas:

- **Component (`comp.go`)** — the file exists and declares a `Comp` struct.
- **Builder (`builder.go`)** — the file exists and declares a `Builder` struct;
  every configurable field has a `With...` setter that returns `Builder`; the
  struct includes at least `Freq` and `Engine`; and a `Build` function exists
  that takes a single `string` argument and returns `*Comp`.
- **Manifest (`manifest.json`)** — the file exists and contains non-empty
  `name`, `ports`, and `parameters` attributes.

```sh
akita check ./mycache
```

Each reported problem is tagged (e.g. `<1>`, `<2b>`, `<3a>`) to indicate which
check failed. On success the command exits 0 with no output.

## Usage in a Project

A typical workflow when adding a component to a simulator:

```sh
# Scaffold the package
akita component --create MyUnit

# ...implement comp.go / builder.go and add manifest.json...

# Verify it conforms to Akita conventions
akita check ./MyUnit
```
