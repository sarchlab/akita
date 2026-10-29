package inspect

import (
	"go/ast"
	"go/types"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"golang.org/x/tools/go/packages"
)

const blueprintPath = "github.com/sarchlab/akita/v5/blueprint"

// A domain is explicitly identified by its Node function returning DomainNode.
// Network constructors return NetworkNode and do not declare domain Ports.
func extractDomainPackage(pkg *packages.Package, index pkgIndex) (schema.Definition, bool, error) {
	obj := pkg.Types.Scope().Lookup("Node")
	if obj == nil {
		return schema.Definition{}, false, nil
	}
	fn, ok := obj.(*types.Func)
	if !ok {
		return schema.Definition{}, false, nil
	}
	sig := fn.Type().(*types.Signature)
	if sig.Results().Len() != 1 {
		return schema.Definition{}, false, nil
	}
	pointer, ok := sig.Results().At(0).Type().(*types.Pointer)
	if !ok || qualifiedTypeName(pointer.Elem()) != blueprintPath+".DomainNode" {
		return schema.Definition{}, false, nil
	}
	specObj := pkg.Types.Scope().Lookup("Spec")
	portsObj := pkg.Types.Scope().Lookup("Ports")
	if specObj == nil || portsObj == nil {
		return schema.Definition{}, false, posErrorf(pkg, obj.Pos(), "domain requires Spec and Ports types")
	}
	if sig.Params().Len() != 1 || !types.Identical(sig.Params().At(0).Type(), specObj.Type()) ||
		sig.TypeParams().Len() != 0 {
		return schema.Definition{}, false, posErrorf(pkg, obj.Pos(),
			"domain Node must have signature func Node(Spec) *blueprint.DomainNode")
	}
	if err := validateDomainConstructor(pkg, fn, specObj.Type(), portsObj.Type()); err != nil {
		return schema.Definition{}, false, err
	}
	if err := validateSpecType(pkg, specObj.Type(), index); err != nil {
		return schema.Definition{}, false, err
	}
	lit, err := defaultSpecLiteral(pkg, specObj.Type(), obj.Pos())
	if err != nil {
		return schema.Definition{}, false, err
	}
	defaults, err := evalStructLiteral(pkg, lit)
	if err != nil {
		return schema.Definition{}, false, err
	}
	def := schema.Definition{SchemaVersion: schema.Version, Kind: schema.KindDomain, Name: pkg.Name,
		Package: pkg.PkgPath}
	if pkg.Module != nil {
		def.Module = pkg.Module.Path
	}
	def.Spec, err = structFields(pkg, specObj.Type(), defaults, index)
	if err != nil {
		return schema.Definition{}, false, err
	}
	if err = validateFieldMetadata(pkg, specObj.Type(), def.Spec); err != nil {
		return schema.Definition{}, false, err
	}
	def.Ports, err = portsOfType(pkg, lit, portsObj.Type(), index)
	return def, err == nil, err
}

// Verify the static constructor convention so inspection cannot silently report
// a different Ports type than the runtime declaration uses.
func validateDomainConstructor(pkg *packages.Package, fn *types.Func, spec, ports types.Type) error {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			node, ok := decl.(*ast.FuncDecl)
			if ok && pkg.TypesInfo.Defs[node.Name] == fn && validDomainBody(pkg, node, fn, spec, ports) {
				return nil
			}
		}
	}
	return posErrorf(pkg, fn.Pos(), "domain Node must directly return blueprint.NewDomain[Spec, Ports](spec, build)")
}
func validDomainBody(pkg *packages.Package, node *ast.FuncDecl, fn *types.Func, spec, ports types.Type) bool {
	if node.Body == nil || len(node.Body.List) != 1 {
		return false
	}
	ret, ok := node.Body.List[0].(*ast.ReturnStmt)
	if !ok || len(ret.Results) != 1 {
		return false
	}
	call, ok := ret.Results[0].(*ast.CallExpr)
	if !ok {
		return false
	}
	indexed, ok := call.Fun.(*ast.IndexListExpr)
	if !ok || len(indexed.Indices) != 2 || len(call.Args) != 2 {
		return false
	}
	selector, ok := indexed.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	callee, ok := pkg.TypesInfo.Uses[selector.Sel].(*types.Func)
	if !ok || callee.Pkg().Path() != blueprintPath || callee.Name() != "NewDomain" {
		return false
	}
	if !types.Identical(pkg.TypesInfo.TypeOf(indexed.Indices[0]), spec) ||
		!types.Identical(pkg.TypesInfo.TypeOf(indexed.Indices[1]), ports) {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	return ok && pkg.TypesInfo.Uses[arg] == fn.Type().(*types.Signature).Params().At(0)
}
