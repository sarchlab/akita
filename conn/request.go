package conn

// A Request is the message element being transferred between compoenents
type Request interface {
	Source() Component

	Destination() Component
}
