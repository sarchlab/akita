# Task tracking architecture

The v5 task layer is `sim/tasks`. Components issue typed commands directly to
one optional Tracker per simulation. It owns open task records, parent
correlation, and port buffer identities. `sim/tracing` observes resolved typed
notifications and owns only persistence or aggregation state.

The only hook adapter converts generic port activity to task lifecycles. Ports
own messages and head transitions; the tracker owns tracing identity. There
is no traceHook dispatch wrapper or generic component hook dependency.

See [the API and contracts](../tasks/README.md). This initial implementation
prioritizes API correctness; sharding, allocation tuning, eager-argument guards,
and comparative performance measurements are follow-up work. Snapshots
and history replay are outside its contract. The simulation checkpoints task
identity and correlation; fresh observers receive full end records for restored
tasks without earlier tags, milestones, or Detail.
