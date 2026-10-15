package inspect

import (
	"go/ast"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// validateInitializers checks optional element methods without executing them.
func validateInitializers(pkg *packages.Package, lit *ast.CompositeLit, args []types.Type) error {
	definition := types.Unalias(pkg.TypesInfo.Types[lit].Type).(*types.Named)
	componentType := definition.Obj().Pkg().Scope().Lookup("Component").Type()
	component, err := types.Instantiate(nil, componentType, args, true)
	if err != nil {
		return posErrorf(pkg, lit.Pos(), "cannot resolve component: %v", err)
	}
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
