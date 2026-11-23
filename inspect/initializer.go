package inspect

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// validateInitializers checks optional element methods without executing them.
func validateInitializers(pkg *packages.Package, component *types.Named, args []types.Type) error {
	for _, i := range []int{1, 4} {
		method, _, _ := types.LookupFieldOrMethod(types.NewPointer(args[i]), false, pkg.Types, "Initialize")
		if method == nil {
			continue
		}
		function, isMethod := method.(*types.Func)
		if !isMethod {
			return posErrorf(pkg, method.Pos(), "%s.Initialize must be a method", args[i])
		}
		sig := function.Type().(*types.Signature)
		if _, pointer := sig.Recv().Type().(*types.Pointer); !pointer {
			return posErrorf(pkg, method.Pos(), "%s.Initialize must use a pointer receiver", args[i])
		}
		if !takesComponent(sig, component) {
			return posErrorf(pkg, method.Pos(), "%s.Initialize must have signature Initialize(*Comp)", args[i])
		}
	}
	return validateCheckpointHook(pkg, component, args[1])
}

// validateCheckpointHook checks State's optional AfterCheckpointLoad method.
// Like the runtime, it consults only method sets, so a serializable State field
// may use the same name.
func validateCheckpointHook(pkg *packages.Package, component *types.Named, state types.Type) error {
	const name = "AfterCheckpointLoad"
	if value := types.NewMethodSet(state).Lookup(pkg.Types, name); value != nil {
		return posErrorf(pkg, value.Obj().Pos(), "%s.%s must use a pointer receiver", state, name)
	}
	method := types.NewMethodSet(types.NewPointer(state)).Lookup(pkg.Types, name)
	if method == nil {
		return nil
	}
	if !takesComponent(method.Type().(*types.Signature), component) {
		return posErrorf(pkg, method.Obj().Pos(), "%s.%s must have signature %s(*Comp)", state, name, name)
	}
	return nil
}

// takesComponent reports whether sig takes only *component and returns nothing.
func takesComponent(sig *types.Signature, component *types.Named) bool {
	return !sig.Variadic() && sig.Params().Len() == 1 && sig.Results().Len() == 0 &&
		types.Identical(sig.Params().At(0).Type(), types.NewPointer(component))
}
