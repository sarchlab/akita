---
sidebar_position: 2
---

# Adding a Cache

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/07_cache/`.
:::

## What You Will Learn

- How to insert `mem/cache/writeback` between a traffic generator and a
  memory controller.
- How to read hit/miss counters and per-bank statistics.
- How latency changes as the working set grows past cache size.

## Outline

1. Take the system from chapter 6 and add a write-back cache in front of
   the memory controller.
2. Configure the cache's geometry (sets, ways, block size) via its Spec.
3. Use an `AddressToPortMapper` to route addresses between cache and
   memory.
4. Run a workload that fits in cache, then one that does not, and compare.
