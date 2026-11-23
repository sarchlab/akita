package inspect

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// validateInitializers checks optional element methods without executing them.
func validateInitializers(pkg *packages.Package, component *types.Named, args []types.Type) error {
	for _, i := range []int{1, 4} {
		if err := validateElementMethod(pkg, component, args[i], "Initialize"); err != nil {
			return err
		}
	}
	return validateElementMethod(pkg, component, args[1], "AfterCheckpointLoad")
}

// validateElementMethod checks that element's optional method, if present, has
// a pointer receiver and takes the component.
func validateElementMethod(
	pkg *packages.Package, component *types.Named, element types.Type, name string,
) error {
	method, _, _ := types.LookupFieldOrMethod(types.NewPointer(element), false, pkg.Types, name)
	if method == nil {
		return nil
	}
	function, isMethod := method.(*types.Func)
	if !isMethod {
		return posErrorf(pkg, method.Pos(), "%s.%s must be a method", element, name)
	}
	sig := function.Type().(*types.Signature)
	if _, pointer := sig.Recv().Type().(*types.Pointer); !pointer {
		return posErrorf(pkg, method.Pos(), "%s.%s must use a pointer receiver", element, name)
	}
	if sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 0 ||
		!types.Identical(sig.Params().At(0).Type(), types.NewPointer(component)) {
		return posErrorf(pkg, method.Pos(), "%s.%s must have signature %s(*Comp)", element, name, name)
	}
	return nil
}
