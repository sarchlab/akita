// Package tracing provides observers for sim/tasks: database recording, time
// statistics, tag counts, and backtraces. Enable the simulation task tracker and
// subscribe observers before running to see starts, or later to receive future events.
// Components call the typed helpers
// in sim/tasks; no task hooks or per-observer parent registries are needed.
package tracing
