package assembly

// Context supplies the simulation's assembly state without exporting setup
// operations on the concrete simulation. Its zero value is ready for setup.
type Context struct{ setup Setup }

func (c *Context) assemblySetup() *Setup { return &c.setup }

// SetupOf returns framework-owned assembly state. Simulation wrappers used in
// tests embed the concrete simulation to preserve this private capability.
func SetupOf(sim any) *Setup {
	return sim.(interface{ assemblySetup() *Setup }).assemblySetup()
}

// Member connects an element to its simulation and optional lifecycle callbacks.
// Only this package can dispatch the callbacks; embedding it exposes no methods.
type Member struct {
	setup      *Setup
	validate   func() error
	initialize func()
}

func NewMember(sim any, validate func() error, initialize func()) Member {
	return Member{setup: SetupOf(sim), validate: validate, initialize: initialize}
}
func (m *Member) assemblyMember() *Member { return m }

func memberOf(value any) *Member {
	if v, ok := value.(interface{ assemblyMember() *Member }); ok {
		return v.assemblyMember()
	}
	return nil
}

// RequireSetup guards changes made by framework elements and assemblers.
func RequireSetup(value any) {
	if m := memberOf(value); m != nil {
		m.setup.RequireSetup()
		return
	}
	SetupOf(value).RequireSetup()
}

// RequireSameSimulation rejects binding or registration of a framework element
// belonging to another simulation, even when task tracking is disabled.
// Custom implementations without Member enforce their own simulation association.
func RequireSameSimulation(sim any, element any) {
	m := memberOf(element)
	if m != nil && m.setup != SetupOf(sim) {
		panic("assembly: element belongs to another simulation")
	}
}
