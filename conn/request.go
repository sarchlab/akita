package conn

// A Request is the message element being transferred between compoenents
type Request struct {
	From Component
	To   Component
}
