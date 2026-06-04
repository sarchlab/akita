// Package control defines the uniform control state model used by every
// memory agent in Akita and provides a reusable conformance harness for
// the mem.ControlReq/ControlRsp protocol.
//
// The protocol verbs themselves live in package mem (mem.ControlCommand,
// mem.ControlReq, mem.ControlRsp). This package adds:
//
//   - State, the shared enumeration of where a memory agent sits in its
//     control lifecycle. Components hold a value of this type in their
//     own state structs so the bookkeeping is uniform and serializable.
//
//   - VerbSupport, a per-component declaration of which verbs the
//     component implements. Verbs that are not declared supported must
//     respond with ControlRsp{Success: false, Error: "unsupported"}.
//
//   - RunContract, a *testing.T-based harness that exercises every verb
//     against a built component over its real Control port. Each
//     component package adds one test that calls RunContract with its
//     build function and its declared VerbSupport.
//
// See CONTROL_PROTOCOL_PLAN.md at the repo root for the verb
// definitions, response timing, and migration plan.
package control
