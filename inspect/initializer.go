package inspect

import (
	"go/types"

	"golang.org/x/tools/go/packages"
)

// validateInitializers checks optional element methods without executing them.
func validateInitializers(pkg *packages.Package, component *types.Named, args []types.Type) error {
	expected := types.NewPointer(component)
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
		if sig.Variadic() || sig.Params().Len() != 1 || sig.Results().Len() != 0 ||
			!types.Identical(sig.Params().At(0).Type(), expected) {
			return posErrorf(pkg, method.Pos(), "%s.Initialize must have signature Initialize(*Comp)", args[i])
		}
	}
	return nil
}
