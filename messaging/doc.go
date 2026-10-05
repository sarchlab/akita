// Package messaging provides messages, ports, connections, and protocols.
//
// A protocol is a set of message types organized into roles, named after the
// package that defines it. Packages that define message types declare their
// protocol once with DefineProtocol, which registers every message type with
// the checkpoint codec, and a component tags each port of its Ports struct
// with the role(s) it speaks, `akita:"role=<protocol>.<role>"`, where
// <protocol> is the defining package's import path.
package messaging
