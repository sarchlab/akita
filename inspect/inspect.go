// Package inspect extracts akita definitions (components, and later other
// kinds) from Go source without executing it, and emits them in the schema of
// package inspect/schema.
//
// A component package declares a package-level var named Definition of a
// component model's Definition type: ticking.Definition, wakeup.Definition, or
// event.Definition (packages modeling/ticking, modeling/wakeup, and
// modeling/event). The same value drives the component at runtime, so the
// emitted schema cannot drift from runtime behavior. From it the inspector
// reads:
//
//   - the model, from the Definition type, and the name, which is the
//     package name because a component type is identified by its package;
//   - the Spec fields, from the Spec type argument, with their docs, units,
//     choices, and `akita:"min=<n>,max=<n>"` tag metadata, and the
//     defaults the DefaultSpec literal assigns. The literal must be keyed and
//     have constant leaves (a slice field's literal lists constants), and
//     Spec fields must be scalars or slices of scalars with distinct JSON
//     names;
//   - the Resources fields, from the Resources type argument, a struct;
//   - the ports, one per field of the Ports type argument: a messaging.Port
//     field is a port, a []messaging.Port field a port group, and an
//     `akita:"role=<protocol>.<role>"` tag names the protocol roles it speaks,
//     each of which must be declared with messaging.DefineProtocol in the
//     component's package or a package it depends on;
//   - the middlewares, one per field of the Middlewares type argument, in the
//     order the component runs them. Each must implement modeling.Middleware.
//
// NewState and NewMiddlewares must name functions, and NewMiddlewares is
// required. A Definition that breaks these rules, or that is not a composite
// literal, is an error rather than a silently incomplete schema.
//
// The inspector loads packages with go/packages, which invokes the Go
// toolchain on the target module but never runs package code. The
// environment is pinned (GOTOOLCHAIN=local, CGO_ENABLED=0) so inspecting an
// untrusted repository does not execute code the repository controls.
package inspect

import (
	"fmt"
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
// all their dependencies: a port's role tag names a protocol that another
// package declares, and field docs come from the packages that declare the
// Spec, Resources, and Middlewares types, so the inspector must read those
// too.
const loadMode = packages.NeedName | packages.NeedFiles |
	packages.NeedCompiledGoFiles | packages.NeedImports | packages.NeedDeps |
	packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo |
	packages.NeedModule

func load(opts Options, patterns ...string) ([]*packages.Package, error) {
	cfg := &packages.Config{
		Mode: loadMode,
		Dir:  opts.Dir,
		// Pin the toolchain so a malicious go.mod cannot select a
		// downloaded toolchain, disable cgo so no C compiler runs on
		// repository-controlled input, and turn off any external
		// go/packages driver named by the environment.
		Env: append(os.Environ(),
			"GOTOOLCHAIN=local", "CGO_ENABLED=0", "GOPACKAGESDRIVER=off"),
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return nil, fmt.Errorf("inspect: loading packages: %w", err)
	}

	return pkgs, nil
}

// pkgIndex maps an import path to the loaded package (with syntax and type
// info), so declarations in other packages (protocols, and the fields of named
// types) can be read.
type pkgIndex map[string]*packages.Package

func indexPackages(roots []*packages.Package) pkgIndex {
	index := pkgIndex{}
	packages.Visit(roots, func(p *packages.Package) bool {
		index[p.PkgPath] = p
		return true
	}, nil)

	return index
}
