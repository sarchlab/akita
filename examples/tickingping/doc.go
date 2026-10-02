// Package tickingping provides an example ticking component that sends and
// receives ping messages.
//
// The component is declared by five structs (Spec, State, Ports, Middlewares,
// and modeling.None for Resources) and the package-level Definition. Every
// tick, its Send middleware sends a due response or the next of Spec.NumPings
// pings to Spec.PingDst, and its ReceiveProcess middleware counts down the
// pings being answered and takes incoming messages. A ping is answered two
// cycles after it arrives.
package tickingping
