# akita — Legacy Command-Line Tool

The `akita` CLI still contains `component --create` and `check`, but these
commands target the older builder-based component layout. The scaffolder
emits references to APIs that are absent from V5, and the checker expects a
hand-written `Comp` and `Builder`. Do not use them to create or validate V5
components. `component-lint`, `component-create`, and `inspect` are not CLI
subcommands.

## Developing V5 components

Follow the [component tutorial](../doc/tutorial/components/01_what_is_a_component.md)
and the runnable [examples](../examples/). A component declares Spec, State,
Resources, Ports, Middlewares, and a package-level `DefaultSpec()` function using one of
`sim/modeling/ticking`, `sim/modeling/wakeup`, or `sim/modeling/event`.

Build with `modeling.MakeBuilder[*Comp](DefaultSpec())`, bind ports, and call
`simulation.Initialize()` before seeding work or running. Add a
`modelingtest.CheckTicking`, `CheckWakeup`, or `CheckEvent` test for the chosen
model. The `inspect` Go package reads component definitions without running
the simulation; it is separate from this legacy CLI.

## Inspecting the existing CLI

```sh
go run ./akita --help
```

This lists the commands actually present in this checkout. Updating or
removing the legacy commands is separate from the V5 documentation repair.
