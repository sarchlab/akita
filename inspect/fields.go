package inspect

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/sarchlab/akita/v5/inspect/schema"
	"github.com/sarchlab/akita/v5/modeling"
)

// unitByType maps fully-qualified named types to the semantic unit they
// imply.
var unitByType = map[string]string{
	"github.com/sarchlab/akita/v5/timing.Freq":           "Hz",
	"github.com/sarchlab/akita/v5/timing.VTimeInSec":     "s",
	"github.com/sarchlab/akita/v5/timing.VTimeInPicoSec": "ps",
}

// structFields describes the exported fields of a Spec or Resources type.
// defaults maps Go field names to the values the definition's DefaultSpec
// literal assigns; nil means the type carries no defaults (Resources).
func structFields(
	pkg *packages.Package, typ types.Type,
	defaults map[string]any, index pkgIndex,
) ([]schema.Field, error) {
	named, _ := typ.(*types.Named)

	st, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}

	docs := fieldDocs(named, index)

	var fields []schema.Field
	for i := range st.NumFields() {
		f := st.Field(i)
		if !f.Exported() {
			continue
		}

		tag := reflect.StructTag(st.Tag(i))
		akitaTag, err := modeling.ParseFieldTag(tag.Get("akita"))
		if err != nil {
			return nil, posErrorf(pkg, f.Pos(),
				"field %s: %v", f.Name(), err)
		}

		field := schema.Field{
			Name:     f.Name(),
			JSONName: jsonName(f.Name(), tag),
			Type:     types.TypeString(f.Type(), types.RelativeTo(pkg.Types)),
			Doc:      docs[f.Name()],
			Unit:     unitByType[qualifiedTypeName(f.Type())],
			Derived:  akitaTag.Derived,
			Min:      akitaTag.Min,
			Max:      akitaTag.Max,
			Choices:  choicesFor(f.Type()),
		}

		if defaults != nil {
			field.Default = fieldDefault(f.Type(), defaults[f.Name()])
		}

		fields = append(fields, field)
	}

	return fields, nil
}

// fieldDefault returns the explicit default from the DefaultSpec literal,
// or the field's zero value when the literal leaves it out. Slices and maps
// default to nil, which the schema omits.
func fieldDefault(typ types.Type, explicit any) any {
	if explicit != nil {
		return explicit
	}

	basic, ok := typ.Underlying().(*types.Basic)
	if !ok {
		return nil
	}

	switch {
	case basic.Info()&types.IsBoolean != 0:
		return false
	case basic.Info()&types.IsUnsigned != 0:
		return uint64(0)
	case basic.Info()&types.IsInteger != 0:
		return int64(0)
	case basic.Info()&types.IsFloat != 0:
		return float64(0)
	case basic.Info()&types.IsString != 0:
		return ""
	default:
		return nil
	}
}

// jsonName mirrors encoding/json's field naming: the json tag's name part,
// the field name when untagged, or "" when the field is excluded.
func jsonName(fieldName string, tag reflect.StructTag) string {
	jsonTag, ok := tag.Lookup("json")
	if !ok {
		return fieldName
	}

	name, _, _ := strings.Cut(jsonTag, ",")
	if name == "-" {
		return ""
	}
	if name == "" {
		return fieldName
	}

	return name
}

// choicesFor lists the declared constants of a named string type, sorted.
// Non-string and unnamed types have no choices.
func choicesFor(typ types.Type) []string {
	named, ok := typ.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return nil
	}

	basic, ok := named.Underlying().(*types.Basic)
	if !ok || basic.Info()&types.IsString == 0 {
		return nil
	}

	scope := named.Obj().Pkg().Scope()

	var choices []string
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if !ok || !types.Identical(c.Type(), named) {
			continue
		}

		choices = append(choices, strings.Trim(c.Val().ExactString(), `"`))
	}

	sort.Strings(choices)

	return choices
}

// fieldDocs collects the doc comments of a named struct type's fields from
// its declaration.
func fieldDocs(named *types.Named, index pkgIndex) map[string]string {
	docs := map[string]string{}

	if named == nil || named.Obj().Pkg() == nil {
		return docs
	}

	declPkg, ok := index[named.Obj().Pkg().Path()]
	if !ok {
		return docs
	}

	st := structTypeSpec(declPkg, named.Obj().Name())
	if st == nil {
		return docs
	}

	for _, field := range st.Fields.List {
		text := ""
		if field.Doc != nil {
			text = strings.TrimSpace(field.Doc.Text())
		} else if field.Comment != nil {
			text = strings.TrimSpace(field.Comment.Text())
		}
		if text == "" {
			continue
		}

		for _, name := range field.Names {
			docs[name.Name] = text
		}
	}

	return docs
}

// structTypeSpec finds the struct type declaration with the given name in
// the package's syntax.
func structTypeSpec(pkg *packages.Package, name string) *ast.StructType {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}

			for _, spec := range gen.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok || ts.Name.Name != name {
					continue
				}

				if st, ok := ts.Type.(*ast.StructType); ok {
					return st
				}
			}
		}
	}

	return nil
}

// qualifiedTypeName returns "importpath.TypeName" for named types, or ""
// otherwise.
func qualifiedTypeName(typ types.Type) string {
	named, ok := typ.(*types.Named)
	if !ok || named.Obj().Pkg() == nil {
		return ""
	}

	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}
