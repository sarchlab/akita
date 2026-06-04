---
slug: /tutorial
sidebar_position: 1
---

# Tutorial Overview

This tutorial teaches Akita through runnable examples. Each chapter is one
self-contained program you can read, run, and modify. The chapters build on
each other, and by the end you will have a working memory-subsystem
simulator and the muscle memory to extend it.

If you are new to Akita, the project's [README](https://github.com/sarchlab/akita)
covers the higher-level pitch — what Akita is, what it gives you, and when
to use it. The rest of this page assumes you know you want to build a
simulation and are ready to start.

## What You Will Build

1. **Components** — the default Akita pattern. Write a component with Spec,
   State, middleware, and ports, then connect two of them.
2. **Event-based simulation** — open the layer underneath: schedule events
   directly, write custom event types, and use event-driven components for
   the idle case.
3. **Building systems** — compose components into a memory subsystem and a
   small network.
4. **Observability and persistence** — trace what your simulation does,
   watch it live, write your own component, and checkpoint to resume long
   runs.

## What You Need

- **Go 1.26 or newer.** Check with `go version`.
- **A clone of the Akita repository.** All examples in the tutorial assume
  you are running them from inside the Akita source tree, where the
  `examples/` directory and module path are already set up.

```bash
git clone https://github.com/sarchlab/akita.git
cd akita
```

Ready? The next chapter writes your first component.
