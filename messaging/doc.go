// Package messaging provides messages, ports, connections, and protocols.
//
// A protocol is a named set of message types organized into roles. Packages
// that define message types declare their protocol once with DefineProtocol,
// which registers every message type with the checkpoint codec, and a
// component tags each port of its Ports struct with the role(s) it speaks,
// `akita:"role=<protocol>/<role>"`.
package messaging
