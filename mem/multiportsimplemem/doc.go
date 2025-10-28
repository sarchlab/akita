package multiportsimplemem

// Package multiportsimplemem provides a simple ticking-based memory model that
// can be shared by multiple agents through independent ports. The component
// serves memory requests with a fixed latency and a configurable number of
// concurrent slots while preserving first-come-first-served ordering across
// all ports.

