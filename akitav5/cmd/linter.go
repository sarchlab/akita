package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type lintIssue struct {
	Rule    string
	Message string
	Pos     token.Position
}

func (i lintIssue) format() string {
	path := i.Pos.Filename
	if path == "" {
		path = "unknown"
	} else if rel, err := filepath.Rel(".", path); err == nil {
		path = rel
	}
	return fmt.Sprintf("%s:%d:%d %s: %s", path, i.Pos.Line, i.Pos.Column, i.Rule, i.Message)
}

func newIssue(fset *token.FileSet, node ast.Node, fallbackPath, rule, msg string) lintIssue {
	var pos token.Position
	if fset != nil && node != nil {
		pos = fset.Position(node.Pos())
	}
	if pos.Filename == "" {
		pos.Filename = fallbackPath
	}
	return lintIssue{Rule: rule, Message: msg, Pos: pos}
}

func issueAtPath(path, rule, msg string) lintIssue {
	return lintIssue{Rule: rule, Message: msg, Pos: token.Position{Filename: path}}
}

// LintComponentFolder runs the component lints against the given folder path.
// It prints findings and returns true if any errors were found.
func LintComponentFolder(folderPath string) bool {
	displayPath := folderPath
	if filepath.IsAbs(folderPath) {
		if wd, err := os.Getwd(); err == nil {
			if rel, err := filepath.Rel(wd, folderPath); err == nil {
				displayPath = rel
			}
		}
	}
	fmt.Println(displayPath)

	hasMarker, markerErr := hasComponentMarker(folderPath)
	if markerErr != nil {
		fmt.Printf("\t%s\n", issueAtPath(folderPath, "Rule 1.2", markerErr.Error()).format())
		return true
	}
	if !hasMarker {
		fmt.Println("\t-- not a component")
		return false
	}

	var issues []lintIssue
	issues = append(issues, checkComponent(folderPath)...)
	issues = append(issues, checkBuilder(folderPath)...)
	issues = append(issues, checkState(folderPath)...)
	issues = append(issues, checkSpec(folderPath)...)

	if len(issues) == 0 {
		fmt.Println("\tOK")
		return false
	}

	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Pos.Filename == issues[j].Pos.Filename {
			if issues[i].Pos.Line == issues[j].Pos.Line {
				return issues[i].Rule < issues[j].Rule
			}
			return issues[i].Pos.Line < issues[j].Pos.Line
		}
		return issues[i].Pos.Filename < issues[j].Pos.Filename
	})

	for _, issue := range issues {
		fmt.Printf("\t%s\n", issue.format())
	}
	return true
}

var builtinTypes = map[string]struct{}{
	"bool": {}, "byte": {}, "complex64": {}, "complex128": {}, "error": {}, "float32": {}, "float64": {},
	"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {}, "rune": {}, "string": {},
	"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {}, "uintptr": {}, "any": {},
}

func checkComponent(folder string) []lintIssue {
	path := filepath.Join(folder, "comp.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []lintIssue{issueAtPath(path, "Rule 1.3", "comp.go file does not exist")}
		}
		return []lintIssue{issueAtPath(path, "Rule 1.3", err.Error())}
	}
	if info.IsDir() {
		return []lintIssue{issueAtPath(path, "Rule 1.3", "comp.go is a directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 1.3", fmt.Sprintf("failed to parse comp.go: %v", err))}
	}

	var issues []lintIssue
	found := false
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			if typeSpec.Name.Name == "Comp" {
				if _, ok := typeSpec.Type.(*ast.StructType); ok {
					found = true
				} else {
					issues = append(issues, newIssue(fset, typeSpec, path, "Rule 1.3", "`Comp` must be a struct"))
				}
			}
		}
	}
	if !found {
		issues = append(issues, newIssue(fset, file.Name, path, "Rule 1.3", "`Comp` struct not found"))
	}
	return issues
}

func checkBuilder(folder string) []lintIssue {
	path := filepath.Join(folder, "builder.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []lintIssue{issueAtPath(path, "Rule 5.1", "builder.go file does not exist")}
		}
		return []lintIssue{issueAtPath(path, "Rule 5.1", err.Error())}
	}
	if info.IsDir() {
		return []lintIssue{issueAtPath(path, "Rule 5.1", "builder.go is a directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 5.1", fmt.Sprintf("failed to parse builder.go: %v", err))}
	}

	var issues []lintIssue
	var builderSpec *ast.TypeSpec
	var builderStruct *ast.StructType
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Builder" {
				continue
			}
			if structType, ok := typeSpec.Type.(*ast.StructType); ok {
				builderSpec = typeSpec
				builderStruct = structType
			}
		}
	}

	if builderStruct == nil {
		return []lintIssue{newIssue(fset, file.Name, path, "Rule 5.1", "`Builder` struct not found")}
	}

	fieldNames := map[string]bool{}
	for _, field := range builderStruct.Fields.List {
		for _, name := range field.Names {
			fieldNames[name.Name] = true
		}
	}
	missing := []string{}
	for _, required := range []string{"Freq", "Engine"} {
		if !fieldNames[required] {
			missing = append(missing, required)
		}
	}
	if len(missing) > 0 {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 5.2", fmt.Sprintf("`Builder` struct must include fields %s", strings.Join(missing, ", "))))
	}

	configured := map[string]bool{}
	for name := range fieldNames {
		configured[name] = false
	}

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}
		recvType := receiverIdent(funcDecl.Recv.List[0].Type)
		if recvType != "Builder" {
			continue
		}
		if strings.HasPrefix(funcDecl.Name.Name, "With") {
			markConfigured(configured, funcDecl)
			if !returnsBuilderValue(funcDecl) {
				issues = append(issues, newIssue(fset, funcDecl, path, "Rule 5.4", "`With` methods must return Builder"))
			}
		}
	}

	var unconfigured []string
	for name, ok := range configured {
		if !ok {
			unconfigured = append(unconfigured, name)
		}
	}
	if len(unconfigured) > 0 {
		issues = append(issues, newIssue(fset, builderStruct, path, "Rule 5.3", fmt.Sprintf("missing `With` setter for field(s): %s", strings.Join(unconfigured, ", "))))
	}

	buildDecl := findBuildFunc(file)
	if buildDecl == nil {
		issues = append(issues, newIssue(fset, file, path, "Rule 5.5", "`Build` method not found"))
		return issues
	}

	if params := buildDecl.Type.Params; params == nil || params.NumFields() != 1 {
		issues = append(issues, newIssue(fset, buildDecl, path, "Rule 5.6", "`Build` must take exactly one argument"))
	} else {
		param := params.List[0]
		if ident, ok := param.Type.(*ast.Ident); !ok || ident.Name != "string" {
			issues = append(issues, newIssue(fset, param, path, "Rule 5.6", "`Build` argument must be of type string"))
		}
	}

	if results := buildDecl.Type.Results; results == nil || results.NumFields() != 1 {
		issues = append(issues, newIssue(fset, buildDecl, path, "Rule 5.7", "`Build` must return *Comp"))
	} else {
		resType := results.List[0].Type
		star, ok := resType.(*ast.StarExpr)
		if !ok {
			issues = append(issues, newIssue(fset, resType, path, "Rule 5.7", "`Build` must return pointer to Comp"))
		} else if ident, ok := star.X.(*ast.Ident); !ok || ident.Name != "Comp" {
			issues = append(issues, newIssue(fset, resType, path, "Rule 5.7", "`Build` must return *Comp"))
		}
	}

	return issues
}

func receiverIdent(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		if ident, ok := t.X.(*ast.Ident); ok {
			return ident.Name
		}
	}
	return ""
}

func markConfigured(configured map[string]bool, funcDecl *ast.FuncDecl) {
	receiverName := ""
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) == 1 {
		if len(funcDecl.Recv.List[0].Names) == 1 {
			receiverName = funcDecl.Recv.List[0].Names[0].Name
		}
	}
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == receiverName {
					configured[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
}

func returnsBuilderValue(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) == 0 {
		return false
	}
	for _, res := range funcDecl.Type.Results.List {
		if ident, ok := res.Type.(*ast.Ident); !ok || ident.Name != "Builder" {
			return false
		}
	}
	return true
}

func findBuildFunc(file *ast.File) *ast.FuncDecl {
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil || funcDecl.Name.Name != "Build" {
			continue
		}
		if funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		if receiverIdent(funcDecl.Recv.List[0].Type) != "Builder" {
			continue
		}
		return funcDecl
	}
	return nil
}

func checkState(folder string) []lintIssue {
	path := filepath.Join(folder, "state.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []lintIssue{issueAtPath(path, "Rule 2.1", err.Error())}
	}
	if info.IsDir() {
		return []lintIssue{issueAtPath(path, "Rule 2.1", "state.go is a directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 2.1", fmt.Sprintf("failed to parse state.go: %v", err))}
	}

	typeDecls := map[string]ast.Expr{}
	var stateStruct *ast.StructType
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeDecls[typeSpec.Name.Name] = typeSpec.Type
			if typeSpec.Name.Name == "state" {
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					stateStruct = structType
				}
			}
		}
	}
	if stateStruct == nil {
		return nil
	}

	var issues []lintIssue
	for _, field := range stateStruct.Fields.List {
		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		}
		violations := collectTypeIssues(field.Type, typeDecls, map[string]bool{})
		for _, v := range violations {
			name := fieldName
			if name == "" {
				name = exprString(fset, field.Type)
			}
			msg := fmt.Sprintf("state.%s %s", name, v.Message)
			issues = append(issues, newIssue(fset, v.Node, path, "Rule 2.1", msg))
		}
	}
	return issues
}

func checkSpec(folder string) []lintIssue {
	path := filepath.Join(folder, "spec.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []lintIssue{issueAtPath(path, "Rule 3.1", err.Error())}
	}
	if info.IsDir() {
		return []lintIssue{issueAtPath(path, "Rule 3.1", "spec.go is a directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 3.1", fmt.Sprintf("failed to parse spec.go: %v", err))}
	}

	typeDecls := map[string]ast.Expr{}
	suffixSpecs := map[string]*ast.StructType{}
	var specStruct *ast.StructType
	var specTypeSpec *ast.TypeSpec

	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			typeDecls[typeSpec.Name.Name] = typeSpec.Type
			if strings.HasSuffix(typeSpec.Name.Name, "Spec") {
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					suffixSpecs[typeSpec.Name.Name] = structType
				}
			}
			if typeSpec.Name.Name == "Spec" {
				specTypeSpec = typeSpec
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					specStruct = structType
				}
			}
		}
	}

	var issues []lintIssue
    if specStruct == nil {
        issues = append(issues, issueAtPath(path, "Rule 3.1", "`Spec` struct not found"))
    } else {
        issues = append(issues, collectStructIssues(specStruct, "Spec.", "Rule 3.2", fset, path, typeDecls)...)
    }

	defaultsFound := false
	validateFound := false

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name == nil {
			continue
		}
		if funcDecl.Recv == nil {
			if funcDecl.Name.Name == "defaults" {
                defaultsFound = true
                if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
                    issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.3", "defaults() must not take parameters"))
                }
                if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
                    issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.3", "defaults() must return Spec"))
                } else if ident, ok := funcDecl.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "Spec" {
                    issues = append(issues, newIssue(fset, funcDecl.Type.Results.List[0].Type, path, "Rule 3.3", "defaults() must return Spec"))
                }
            }
            continue
		}
		recvType := receiverIdent(funcDecl.Recv.List[0].Type)
		if recvType == "Spec" && funcDecl.Name.Name == "validate" {
			validateFound = true
            if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
                issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.4", "validate() must not take parameters"))
            }
            if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
                issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.4", "validate() must return error"))
            } else if ident, ok := funcDecl.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "error" {
                issues = append(issues, newIssue(fset, funcDecl.Type.Results.List[0].Type, path, "Rule 3.4", "validate() must return error"))
            }
        }
    }

    if !defaultsFound {
        issues = append(issues, issueAtPath(path, "Rule 3.3", "defaults() function not found"))
    }
    if !validateFound {
        if specTypeSpec != nil {
            issues = append(issues, newIssue(fset, specTypeSpec, path, "Rule 3.4", "(Spec) validate() method not found"))
        } else {
            issues = append(issues, issueAtPath(path, "Rule 3.4", "(Spec) validate() method not found"))
        }
    }

    for name, structType := range suffixSpecs {
        if name == "Spec" {
            continue
        }
        issues = append(issues, collectStructIssues(structType, name+".", "Rule 3.2", fset, path, typeDecls)...)
    }

	return issues
}

func collectStructIssues(structType *ast.StructType, prefix, rule string, fset *token.FileSet, path string, typeDecls map[string]ast.Expr) []lintIssue {
	var issues []lintIssue
	for _, field := range structType.Fields.List {
		fieldName := ""
		if len(field.Names) > 0 {
			fieldName = field.Names[0].Name
		}
		violations := collectTypeIssues(field.Type, typeDecls, map[string]bool{})
		for _, v := range violations {
			name := fieldName
			if name == "" {
				name = exprString(fset, field.Type)
			}
			msg := fmt.Sprintf("%s%s %s", prefix, name, v.Message)
			issues = append(issues, newIssue(fset, v.Node, path, rule, msg))
		}
	}
	return issues
}

type typeIssue struct {
	Node    ast.Node
	Message string
}

func collectTypeIssues(expr ast.Expr, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
	switch t := expr.(type) {
	case *ast.Ident:
		if _, ok := builtinTypes[t.Name]; ok {
			return nil
		}
		if decl, ok := typeDecls[t.Name]; ok {
			if visiting[t.Name] {
				return nil
			}
			visiting[t.Name] = true
			nested := collectTypeIssues(decl, typeDecls, visiting)
			visiting[t.Name] = false
			for i := range nested {
				nested[i].Message = fmt.Sprintf("via type %s, %s", t.Name, nested[i].Message)
			}
			return nested
		}
		return []typeIssue{{Node: t, Message: fmt.Sprintf("uses external type %s", t.Name)}}
	case *ast.StructType:
		var issues []typeIssue
		for _, field := range t.Fields.List {
			fieldName := ""
			if len(field.Names) > 0 {
				fieldName = field.Names[0].Name
			}
			nested := collectTypeIssues(field.Type, typeDecls, visiting)
			for i := range nested {
				if fieldName != "" {
					nested[i].Message = fmt.Sprintf("field %s %s", fieldName, nested[i].Message)
				} else {
					nested[i].Message = fmt.Sprintf("embedded field %s", nested[i].Message)
				}
				issues = append(issues, nested[i])
			}
		}
		return issues
	case *ast.ArrayType:
		nested := collectTypeIssues(t.Elt, typeDecls, visiting)
		for i := range nested {
			nested[i].Message = fmt.Sprintf("array element %s", nested[i].Message)
		}
		return nested
	case *ast.MapType:
		var issues []typeIssue
		for _, issue := range collectTypeIssues(t.Key, typeDecls, visiting) {
			issue.Message = fmt.Sprintf("map key %s", issue.Message)
			issues = append(issues, issue)
		}
		for _, issue := range collectTypeIssues(t.Value, typeDecls, visiting) {
			issue.Message = fmt.Sprintf("map value %s", issue.Message)
			issues = append(issues, issue)
		}
		return issues
	case *ast.StarExpr:
		return []typeIssue{{Node: t, Message: "contains pointer type"}}
	case *ast.ChanType:
		return []typeIssue{{Node: t, Message: "contains channel type"}}
	case *ast.FuncType:
		return []typeIssue{{Node: t, Message: "contains function type"}}
	case *ast.InterfaceType:
		return []typeIssue{{Node: t, Message: "contains interface type"}}
	case *ast.ParenExpr:
		return collectTypeIssues(t.X, typeDecls, visiting)
	case *ast.SelectorExpr:
		return nil
	default:
		return []typeIssue{{Node: t, Message: fmt.Sprintf("uses unsupported type %T", t)}}
	}
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	if fset == nil {
		return ""
	}
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

func hasComponentMarker(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return false, err
		}
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "//akita:component") {
				return true, nil
			}
		}
		if err := scanner.Err(); err != nil {
			return false, err
		}
	}
	return false, nil
}
