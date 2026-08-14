// Package inspect extracts akita definitions (components, and later other
// kinds) from Go source without executing it. It statically evaluates the
// package-level DefineComponent call that also drives the component at
// runtime, so the emitted schema cannot drift from runtime behavior.
//
// The inspector loads packages with go/packages, which invokes the Go
// toolchain on the target module but never runs package code. The
// environment is pinned (GOTOOLCHAIN=local, CGO_ENABLED=0) so inspecting an
// untrusted repository does not execute code the repository controls.
package inspect

import (
	"fmt"
	"go/ast"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
)

// Options configures an inspection run.
type Options struct {
	// Dir is the working directory for package loading. Empty means the
	// current directory.
	Dir string
}

// Inspect loads the packages matching the given patterns (e.g. "./..." or an
// import path) and extracts every definition found. Packages that contain no
// definition are skipped silently; packages whose definition is invalid or
// not statically analyzable contribute an error. Definitions are sorted by
// package path.
func Inspect(opts Options, patterns ...string) ([]schema.Definition, []error) {
	pkgs, err := load(opts, patterns...)
	if err != nil {
		return nil, []error{err}
	}

	index := indexPackages(pkgs)

	var defs []schema.Definition
	var errs []error

	for _, pkg := range pkgs {
		if len(pkg.Errors) > 0 {
			for _, e := range pkg.Errors {
				errs = append(errs, fmt.Errorf("%s: %w", pkg.PkgPath, e))
			}
			continue
		}

		def, ok, err := extractPackage(pkg, index)
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if ok {
			defs = append(defs, def)
		}
	}

	sort.Slice(defs, func(i, j int) bool {
		return defs[i].Package < defs[j].Package
	})

	return defs, errs
}

// loadMode requests syntax and type information for the target packages and
// all their dependencies: role identifiers in a definition resolve to var
// declarations in other packages, so the inspector must read those too.
const loadMode = packages.NeedName | packages.NeedFiles |
	packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps |
	packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
	packages.NeedModule

func load(opts Options, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  opts.Dir,
		// Pin the toolchain so a malicious go.mod cannot select a
		// downloaded toolchain, and disable cgo so no C compiler runs on
		// repository-controlled input.
		Env: append(os.Environ(), "GOTOOLCHAIN=local", "CGO_ENABLED=0"),
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("inspect: loading packages: %w", err)
	}

	return pkgs, nil
}

// pkgIndex locates the loaded package (with syntax and type info) that
// defines a given types object, so cross-package references (roles,
// protocols, named types) can be followed to their declarations.
type pkgIndex map[string]*packages.Package

func indexPackages(roots []*packages.Package) pkgIndex {
	index := pkgIndex{}
	packages.Visit(roots, func(p *packages.Package) bool {
		index[p.PkgPath] = p
		return true
	}, nil)

	return index
}

// declOf finds the ValueSpec and initializer expression for a package-level
// var with the given name in pkg. It returns nil if not found.
func declOf(pkg *packages.Package, name string) ast.Expr {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}

			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}

				for i, ident := range vs.Names {
					if ident.Name == name && i < len(vs.Values) {
						return vs.Values[i]
					}
				}
			}
		}
	}

	return nil
}
