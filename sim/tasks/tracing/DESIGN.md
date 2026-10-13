# Task tracking architecture

The v5 task layer is `sim/tasks`. Components and ports issue typed commands to
one optional Tracker per simulation. It owns open task records and request-parent
correlation. Components and ports retain their own task IDs. `sim/tasks/tracing`
observes resolved events and owns persistence or aggregation state.

`twowaybuffered.Port` stores task IDs and milestone boundaries in its private
buffer entries. It starts and ends tasks directly and checkpoints those entries
with their messages. Generic hooks are independent of this lifecycle; no hook
adapter is required.

See [the API and contracts](../README.md). This initial implementation
prioritizes API correctness; sharding, allocation tuning, eager-argument guards,
and comparative performance measurements are follow-up work. Subscriptions may
join mid-run, but there is no snapshot or history replay. Fresh observers receive
complete end records for restored or already-running tasks. Statistical tracers
count their full intervals. BusyTimeTracer retains the union of completed
intervals so late ends can overlap earlier results without double counting.
