---
sidebar_position: 1
---

# Recording Traces to a Database

The **Getting Information from a Simulation** section introduced tasks and
in-memory tracers
(`tracing.StartTask` / `EndTask`, `BusyTimeTracer`, `AverageTimeTracer`).
This chapter builds on that to *persist* a full trace for offline analysis,
rather than reading a single number at the end of a run.

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/09_tracing/`.
:::

## What You Will Learn

- The `DBTracer` and the `datarecording` package, which write every task
  and milestone to a SQLite database.
- Selective tracing with `StartTracing` / `StopTracing` to capture only a
  window of interest.
- How to open the resulting database and query the recorded tasks.

## Outline

1. Take the cache + memory system from the Building Systems section.
2. Wrap each request in a tracing task so the full lifecycle is recorded
   (the task API from Getting Information from a Simulation).
3. Attach a `DBTracer` backed by a `datarecording` recorder to the
   simulation.
4. Run the workload, open the resulting SQLite file, and query it. The next
   chapter, *Live Monitoring*, visualises the same data in Daisen.
