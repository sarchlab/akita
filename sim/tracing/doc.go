// Package tracing provides observers for sim/tasks: database recording, time
// statistics, tag counts, and backtraces. Enable the simulation task tracker and
// subscribe observers before the engine runs. Components call the typed helpers
// in sim/tasks; no task hooks or per-observer parent registries are needed.
package tracing
