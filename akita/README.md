# akita — Command-Line Shell

The `akita` executable and its installation path remain available in V5.
Running `akita` or `akita --help` displays help and the CLI's current status.

The old generator, templates, and layout checker have been removed. Their
command spellings remain as deprecated compatibility stubs:

- `akita component --create NAME` reports that generation is unavailable.
- `akita check PATH` reports that component checking is unavailable.

Both commands print an error to stderr and exit with status 1. They create or
modify no files and perform no validation. Their `--help` options still work.
Keeping these spellings preserves command recognition, not their old behavior.

A replacement CLI is planned for [V5.1 (#523)](https://github.com/sarchlab/akita/issues/523).
The legacy syntax is not the permanent interface for that rewrite; migration
and any compatibility aliases will be documented there. `component-lint`,
`component-create`, and `inspect` are not CLI subcommands.

## Developing V5 components

Follow the [component tutorial](../doc/tutorial/components/01_what_is_a_component.md)
and the runnable [examples](../examples/). A component declares Spec, State,
Resources, Ports, Middlewares, and a package-level `DefaultSpec()` function using one of
`sim/modeling/ticking`, `sim/modeling/wakeup`, or `sim/modeling/event`.

Build with `modeling.MakeBuilder[*Comp](DefaultSpec())`, bind ports, and call
`simulation.Initialize()` before seeding work or running. Add a
`modelingtest.CheckTicking`, `CheckWakeup`, or `CheckEvent` test for the chosen
model. The `inspect` Go package reads component definitions without running
the simulation; it is separate from this CLI shell.

## Installing and inspecting the CLI

```sh
go install github.com/sarchlab/akita/v5/akita@latest
akita --help
```

From a source checkout:

```sh
go run ./akita --help
```

The command implementation lives in `akita/internal/cmd` and is not a public Go API.
