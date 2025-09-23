// Package tracing layers structured task and request tracing on top of the
// shared instrumentation/hooking primitives.
//
// It translates low-level hook events into a three-tier model: generic hooks
// originate from instrumentation/hooking, this package adds task/tag/milestone
// helpers, and higher-level helpers map request lifecycles onto those tasks.
package tracing
