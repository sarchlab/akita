package hookable

const (
	Before = iota
	After
)

type Hookable interface {
	Hook()
}
