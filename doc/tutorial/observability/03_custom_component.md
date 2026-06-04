---
sidebar_position: 3
---

# Writing Your Own Component

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/11_custom_component/`.
:::

## What You Will Learn

- Designing a component from scratch: what goes in Spec, what goes in
  State, how to split middleware.
- The builder convention enforced across the Akita codebase
  (`MakeBuilder` + `WithRegistrar` + `WithSpec` + `Build` +
  `DefaultSpec`).
- Writing a unit test that drives the component's ports directly
  without a network.

## Outline

1. Pick a small but realistic component — a round-robin arbiter — and
   sketch its responsibilities.
2. Define Spec, State, and the messages it handles.
3. Implement two middlewares: one to grant access, one to forward
   granted traffic.
4. Write a builder following the Akita convention.
5. Write a standalone unit test using `modeling.NewStandaloneRegistrar`
   and a `noopConn` to drive ports without a real connection.
