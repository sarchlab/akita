package messaging

// AnyProtocol is the protocol of a port that takes messages of every protocol,
// such as a message sink that consumes whatever arrives. It has one role,
// AnyRole, which lists no messages: a port that speaks it is compatible with
// every role of every protocol. Tag such a port
//
//	`akita:"role=github.com/sarchlab/akita/v5/simulation/messaging.any"`
//
// A port without a role tag declares nothing about what it speaks; a port
// with AnyRole declares that it speaks anything.
var (
	AnyProtocol = DefineProtocol(RoleDef{Name: "any"})
	AnyRole     = AnyProtocol.Role("any")
)
