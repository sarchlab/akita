package simulation

// EntityKind identifies the top-level category of a simulation entity.
type EntityKind string

const (
	// EntityKindComponent identifies a registered component.
	EntityKindComponent EntityKind = "component"

	// EntityKindPort identifies a registered port.
	EntityKindPort EntityKind = "port"

	// EntityKindConnection identifies a registered connection.
	EntityKindConnection EntityKind = "connection"

	// EntityKindResource identifies a registered shared-state resource.
	EntityKindResource EntityKind = "resource"
)

// Entity is a stable reference to a registered simulation object.
type Entity struct {
	Kind EntityKind
	Name string

	// Type is optional type metadata within the entity kind. Resources use this
	// for their resource kind, such as "mem.Storage".
	Type string
}

func newEntityNameIndex() map[EntityKind]map[string]int {
	return map[EntityKind]map[string]int{
		EntityKindComponent:  make(map[string]int),
		EntityKindPort:       make(map[string]int),
		EntityKindConnection: make(map[string]int),
		EntityKindResource:   make(map[string]int),
	}
}

func (s *Simulation) registerEntity(entity Entity) {
	if entity.Kind == "" {
		panic("entity kind cannot be empty")
	}

	if entity.Name == "" {
		panic("entity name cannot be empty")
	}

	if s.entityNameIndex == nil {
		s.entityNameIndex = newEntityNameIndex()
	}

	kindIndex, found := s.entityNameIndex[entity.Kind]
	if !found {
		kindIndex = make(map[string]int)
		s.entityNameIndex[entity.Kind] = kindIndex
	}

	if _, found := kindIndex[entity.Name]; found {
		panic(string(entity.Kind) + " " + entity.Name + " already registered")
	}

	s.entities = append(s.entities, entity)
	kindIndex[entity.Name] = len(s.entities) - 1
}

func componentEntity(name string) Entity {
	return Entity{
		Kind: EntityKindComponent,
		Name: name,
	}
}

func portEntity(name string) Entity {
	return Entity{
		Kind: EntityKindPort,
		Name: name,
	}
}

func connectionEntity(name string) Entity {
	return Entity{
		Kind: EntityKindConnection,
		Name: name,
	}
}

func resourceEntity(resource Resource) Entity {
	return Entity{
		Kind: EntityKindResource,
		Name: resource.Name(),
		Type: resource.Kind(),
	}
}
