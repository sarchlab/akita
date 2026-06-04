---
sidebar_position: 3
---

# A Small NoC

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/08_noc/`.
:::

## What You Will Learn

- Replacing `directconnection` with a real network topology.
- Building a small mesh with `noc/networking/mesh`.
- Routing memory requests across the network.

## Outline

1. Two compute "tiles" (each: traffic gen + cache) and one memory
   controller, arranged on a 2x2 mesh.
2. Configure routing tables.
3. Run mixed traffic and observe contention.
4. Compare end-to-end latency with the zero-latency direct connection.
