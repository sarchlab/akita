---
sidebar_position: 4
---

# Checkpoint and Restore

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/12_checkpoint/`.
:::

## What You Will Learn

- What **quiescence** means and how to drive a simulation into it before
  saving.
- Using `simulation.Save(path)` to persist a snapshot.
- Using `simulation.Load(path)` to resume from that snapshot.
- Why every Spec and State field has a `json` tag.

## Outline

1. Run the cache + memory system from chapter 7 for a fixed number of
   cycles, then drain all in-flight traffic to reach quiescence.
2. Save the snapshot to disk.
3. In a fresh process, build the same topology, load the snapshot, kick
   off the workload again, and verify the run continues correctly.
4. Inspect the JSON files in the snapshot to see Spec/State persisted on
   disk.
