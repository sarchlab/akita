---
sidebar_position: 2
---

# Live Monitoring with Daisen and Akita RTM

:::note Coming soon
This chapter is being written. The runnable example will live at
`examples/10_monitoring/`.
:::

## What You Will Learn

- Enabling **Akita RTM** (the live in-situ monitor) on a running
  simulation and inspecting state in the browser.
- Recording a trace for offline viewing in **Daisen**.
- Reading a Daisen timeline to find bottlenecks.

## Outline

1. Take the cache + memory + NoC system from chapter 8.
2. Start the RTM web server alongside the simulation. Open it in a
   browser, navigate the component tree, inspect ports.
3. Enable visual tracing (`WithVisTracingOnStart`) and save the trace.
4. Open the trace in Daisen, walk through one full request's lifecycle.
