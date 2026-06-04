---
sidebar_position: 1
---

# Memory: Traffic Generator + Ideal Controller

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/06_memory/`.
:::

## What You Will Learn

- How to use Akita's `mem.ReadReq` and `mem.WriteReq` protocol.
- How to plug a component into `idealmemcontroller`.
- How to read and write memory and verify the result.

## Outline

1. Define a small traffic-generator component that issues a sequence of
   reads and writes.
2. Build an `idealmemcontroller` with a fixed latency and a `mem.Storage`
   backing.
3. Connect them with `directconnection`.
4. Run, observe the responses, and verify that writes are readable.
