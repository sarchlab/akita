package modeling

// MakeBuilder creates a typed builder for a component pointer type, using the
// supplied configuration. For example:
//
//	modeling.MakeBuilder[*rob.Comp](rob.DefaultSpec()).WithSimulation(s).Build("ROB")
//
// C is normally *package.Comp; its model supplies MakeBuilder. Spec and the
// concrete builder type are inferred, so callers never repeat the five element
// types. A DefaultSpec function is called explicitly, not discovered at runtime.
func MakeBuilder[C interface{ MakeBuilder(S) B }, S, B any](spec S) B {
	var component C
	return component.MakeBuilder(spec)
}
