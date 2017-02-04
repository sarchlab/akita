// Package conn defines the communication mechanism that is used in
// Yaotsu.
//
// To make the communication model general, Yaotsu define all the units
// that many need to communication as Compoenent. Components define Sockets
// as connecting points to the outside world. A Connection object can "Plugin"
// into the Socket to deliever the message to the destination. Therefore,
// following this model, users can really make all the configureation and
// wiring flexible.
//
// There are three different types of connections are provided, DirectConn,
// FixedLatencyConn, and NetworkedConn.
//
// All the message that sent from one compoenent to another is called Request.
package conn
