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

	// EntityKindEngine identifies the simulation engine singleton.
	EntityKindEngine EntityKind = "engine"

	// EntityKindIDGenerator identifies the global ID generator singleton.
	EntityKindIDGenerator EntityKind = "id-generator"
)

// Reserved names for the singleton runtime entities. They share the same flat
// namespace as user entities, so user objects must not use these names.
const (
	engineEntityName      = "engine"
	idGeneratorEntityName = "id-generator"
)

// Entity is a stable reference to a registered simulation object. It is the
// common vocabulary the global state manager uses for every registered runtime
// object, regardless of its concrete kind.
type Entity struct {
	Kind EntityKind
	Name string

	// Type is optional type metadata within the entity kind. Resources use this
	// for their resource kind, such as "mem.Storage".
	Type string
}

// registerEntity records an entity and the live object it refers to in the
// single, flat entity inventory. Names must be globally unique across all
// kinds, which is what makes GetStateByName well defined.
func (s *Simulation) registerEntity(entity Entity, object any) {
	if entity.Kind == "" {
		panic("entity kind cannot be empty")
	}

	if entity.Name == "" {
		panic("entity name cannot be empty")
	}

	if s.entityByName == nil {
		s.entityByName = make(map[string]int)
	}

	if _, found := s.entityByName[entity.Name]; found {
		panic(string(entity.Kind) + " " + entity.Name + " already registered")
	}

	s.entities = append(s.entities, entity)
	s.entityObjects = append(s.entityObjects, object)
	s.entityByName[entity.Name] = len(s.entities) - 1
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
