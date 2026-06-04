---
sidebar_position: 1
---

# Tracing and Metrics

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/09_tracing/`.
:::

## What You Will Learn

- The task-based tracing API: `tracing.StartTask`, `AddTaskStep`,
  `EndTask`.
- Built-in tracers: `BusyTimeTracer`, `AverageTimeTracer`,
  `StepCountTracer`, `DBTracer`.
- Recording per-component metrics into the SQLite database via the
  `datarecording` package.

## Outline

1. Take the cache + memory system from chapter 7.
2. Wrap each request in a tracing task so the full lifecycle is recorded.
3. Attach a `BusyTimeTracer` to the cache and a `DBTracer` to the whole
   simulation.
4. Run the workload, open the resulting SQLite file, and query it.
