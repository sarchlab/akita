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
	if pos.Line == 0 {
		pos.Line = 1
	}
	if pos.Column == 0 {
		pos.Column = 1
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

func validateBuilderFile(folder string) (string, error) {
	path := filepath.Join(folder, "builder.go")
	info, err := os.Stat(path)
	if err != nil {
		return path, err
	}
	if info.IsDir() {
		return path, fmt.Errorf("builder.go is a directory")
	}
	return path, nil
}

func parseBuilderFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse builder.go: %v", err)
	}
	return fset, file, nil
}

func extractBuilderStruct(file *ast.File) (*ast.TypeSpec, *ast.StructType) {
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
				return typeSpec, structType
			}
		}
	}
	return nil, nil
}

func extractBuilderFields(builderStruct *ast.StructType) (map[string]bool, map[string]ast.Expr, map[string]bool) {
	fieldNames := map[string]bool{}
	fieldTypes := map[string]ast.Expr{}
	configured := map[string]bool{}

	for _, field := range builderStruct.Fields.List {
		for _, name := range field.Names {
			fieldNames[name.Name] = true
			fieldTypes[name.Name] = field.Type
		}
	}

	for name := range fieldNames {
		if name != "spec" && name != "simulation" {
			configured[name] = false
		}
	}

	return fieldNames, fieldTypes, configured
}

func processWithMethods(
	file *ast.File, fset *token.FileSet, path string, configured map[string]bool,
) ([]lintIssue, map[string]map[string]bool, map[string]*ast.FuncDecl) {
	var issues []lintIssue
	withAssignments := map[string]map[string]bool{}
	withDecls := map[string]*ast.FuncDecl{}

	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}
		recvType := receiverIdent(funcDecl.Recv.List[0].Type)
		if recvType != "Builder" || !strings.HasPrefix(funcDecl.Name.Name, "With") {
			continue
		}

		assignments := assignedBuilderFields(funcDecl)
		withAssignments[funcDecl.Name.Name] = assignments
		withDecls[funcDecl.Name.Name] = funcDecl

		for field := range assignments {
			if _, ok := configured[field]; ok {
				configured[field] = true
			}
		}

		if !returnsBuilderValue(funcDecl) {
			message := "`With` methods must return Builder"
			issues = append(issues, newIssue(fset, funcDecl, path, "Rule 4.4", message))
		}
	}

	return issues, withAssignments, withDecls
}

func validateUnconfiguredFields(
	fset *token.FileSet, builderStruct *ast.StructType, path string, configured map[string]bool,
) []lintIssue {
	var unconfigured []string
	for name, ok := range configured {
		if !ok {
			unconfigured = append(unconfigured, name)
		}
	}
	if len(unconfigured) > 0 {
		message := fmt.Sprintf("missing `With` setter for field(s): %s", strings.Join(unconfigured, ", "))
		return []lintIssue{newIssue(fset, builderStruct, path, "Rule 4.4", message)}
	}
	return nil
}

func validateBuildMethod(
	file *ast.File, fset *token.FileSet, path string,
) []lintIssue {
	var issues []lintIssue
	buildDecl := findBuildFunc(file)
	if buildDecl == nil {
		return []lintIssue{newIssue(fset, file, path, "Rule 4.5", "`Build` method not found")}
	}

	if params := buildDecl.Type.Params; params == nil || params.NumFields() != 1 {
		message := "`Build` must take exactly one argument"
		issues = append(issues, newIssue(fset, buildDecl, path, "Rule 4.5", message))
	} else {
		param := params.List[0]
		if ident, ok := param.Type.(*ast.Ident); !ok || ident.Name != "string" {
			message := "`Build` argument must be of type string"
			issues = append(issues, newIssue(fset, param, path, "Rule 4.5", message))
		}
	}

	if results := buildDecl.Type.Results; results == nil || results.NumFields() != 1 {
		message := "`Build` must return *Comp"
		issues = append(issues, newIssue(fset, buildDecl, path, "Rule 4.5", message))
	} else {
		resType := results.List[0].Type
		star, ok := resType.(*ast.StarExpr)
		if !ok {
			message := "`Build` must return pointer to Comp"
			issues = append(issues, newIssue(fset, resType, path, "Rule 4.5", message))
		} else if ident, ok := star.X.(*ast.Ident); !ok || ident.Name != "Comp" {
			message := "`Build` must return *Comp"
			issues = append(issues, newIssue(fset, resType, path, "Rule 4.5", message))
		}
	}

	if !buildCallsSpecValidate(buildDecl) {
		message := "`Build` must validate the spec"
		issues = append(issues, newIssue(fset, buildDecl, path, "Rule 4.6", message))
	}

	return issues
}

func checkBuilder(folder string) []lintIssue {
	path, err := validateBuilderFile(folder)
	if err != nil {
		if os.IsNotExist(err) {
			return []lintIssue{issueAtPath(path, "Rule 4.1", "builder.go file does not exist")}
		}
		return []lintIssue{issueAtPath(path, "Rule 4.1", err.Error())}
	}

	fset, file, err := parseBuilderFile(path)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 4.1", err.Error())}
	}

	builderSpec, builderStruct := extractBuilderStruct(file)
	if builderStruct == nil {
		return []lintIssue{newIssue(fset, file.Name, path, "Rule 4.1", "`Builder` struct not found")}
	}

	fieldNames, fieldTypes, configured := extractBuilderFields(builderStruct)
	_ = fieldNames // mark as used to avoid linter warning

	var issues []lintIssue
	withIssues, withAssignments, withDecls := processWithMethods(file, fset, path, configured)
	issues = append(issues, withIssues...)

	unconfiguredIssues := validateUnconfiguredFields(fset, builderStruct, path, configured)
	issues = append(issues, unconfiguredIssues...)

	simRequirements := validateSimulationRequirements(fset, path, builderSpec, fieldTypes["simulation"], 
		withDecls["WithSimulation"], withAssignments["WithSimulation"])
	issues = append(issues, simRequirements...)

	specRequirements := validateSpecRequirements(fset, path, builderSpec, fieldTypes["spec"], 
		withDecls["WithSpec"], withAssignments["WithSpec"])
	issues = append(issues, specRequirements...)

	buildIssues := validateBuildMethod(file, fset, path)
	issues = append(issues, buildIssues...)

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

func assignedBuilderFields(funcDecl *ast.FuncDecl) map[string]bool {
	assigned := map[string]bool{}
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
					assigned[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
	return assigned
}

func validateSimulationRequirements(
	fset *token.FileSet, path string, builderSpec *ast.TypeSpec, field ast.Expr,
	withDecl *ast.FuncDecl, assignments map[string]bool,
) []lintIssue {
	var issues []lintIssue
	if field == nil {
		message := "`Builder` struct must include `simulation` field"
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.2", message))
		return issues
	}

	typeStr := exprString(fset, field)
	if typeStr != "*simv5.Simulation" && typeStr != "simv5.Simulation" {
		message := "`simulation` field must be of type *simv5.Simulation"
		issues = append(issues, newIssue(fset, field, path, "Rule 4.2", message))
	}

	if withDecl == nil {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.2", "`WithSimulation` method not found"))
		return issues
	}

	if assignments == nil || !assignments["simulation"] {
		message := "`WithSimulation` must assign to the `simulation` field"
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.2", message))
	}

	if withDecl.Type.Params == nil || withDecl.Type.Params.NumFields() != 1 {
		message := "`WithSimulation` must take exactly one simv5.Simulation argument"
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.2", message))
	} else {
		paramType := exprString(fset, withDecl.Type.Params.List[0].Type)
		if paramType != "*simv5.Simulation" && paramType != "simv5.Simulation" {
			message := "`WithSimulation` argument must be simv5.Simulation"
			issues = append(issues, newIssue(fset, withDecl.Type.Params.List[0].Type, path, "Rule 4.2", message))
		}
	}

	return issues
}

func validateSpecRequirements(
	fset *token.FileSet, path string, builderSpec *ast.TypeSpec, field ast.Expr,
	withDecl *ast.FuncDecl, assignments map[string]bool,
) []lintIssue {
	var issues []lintIssue
	if field == nil {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.3", "`Builder` struct must include `spec` field"))
		return issues
	}

	typeStr := exprString(fset, field)
	if typeStr != "Spec" && typeStr != "*Spec" {
		issues = append(issues, newIssue(fset, field, path, "Rule 4.3", "`spec` field must use Spec type"))
	}

	if withDecl == nil {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.3", "`WithSpec` method not found"))
		return issues
	}

	if assignments == nil || !assignments["spec"] {
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.3", "`WithSpec` must assign to the `spec` field"))
	}

	if withDecl.Type.Params == nil || withDecl.Type.Params.NumFields() != 1 {
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.3", "`WithSpec` must take exactly one Spec argument"))
	} else {
		paramType := exprString(fset, withDecl.Type.Params.List[0].Type)
		if paramType != "Spec" && paramType != "*Spec" {
			message := "`WithSpec` argument must be Spec"
			issues = append(issues, newIssue(fset, withDecl.Type.Params.List[0].Type, path, "Rule 4.3", message))
		}
	}

	return issues
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

func getReceiverName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) != 1 {
		return ""
	}
	if len(funcDecl.Recv.List[0].Names) != 1 {
		return ""
	}
	return funcDecl.Recv.List[0].Names[0].Name
}

func processAssignment(assign *ast.AssignStmt, receiverName string, aliases map[string]bool) {
	for i, lhs := range assign.Lhs {
		ident, ok := lhs.(*ast.Ident)
		if !ok || i >= len(assign.Rhs) {
			continue
		}
		rhs := assign.Rhs[i]
		switch val := rhs.(type) {
		case *ast.SelectorExpr:
			if recv, ok := val.X.(*ast.Ident); ok && recv.Name == receiverName && val.Sel.Name == "spec" {
				aliases[ident.Name] = true
			}
		case *ast.Ident:
			if aliases[val.Name] {
				aliases[ident.Name] = true
			}
		}
	}
}

func isValidateCall(call *ast.CallExpr) *ast.SelectorExpr {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel == nil {
		return nil
	}
	if sel.Sel.Name != "validate" && sel.Sel.Name != "Validate" {
		return nil
	}
	return sel
}

func isSpecValidateCall(sel *ast.SelectorExpr, receiverName string, aliases map[string]bool) bool {
	switch recv := sel.X.(type) {
	case *ast.SelectorExpr:
		if ident, ok := recv.X.(*ast.Ident); ok && ident.Name == receiverName && recv.Sel.Name == "spec" {
			return true
		}
	case *ast.Ident:
		if aliases[recv.Name] {
			return true
		}
	}
	return false
}

func buildCallsSpecValidate(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Body == nil {
		return false
	}

	receiverName := getReceiverName(funcDecl)
	aliases := map[string]bool{}
	found := false

	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		if found {
			return false
		}

		if assign, ok := n.(*ast.AssignStmt); ok {
			processAssignment(assign, receiverName, aliases)
			return true
		}

		if call, ok := n.(*ast.CallExpr); ok {
			if sel := isValidateCall(call); sel != nil {
				if isSpecValidateCall(sel, receiverName, aliases) {
					found = true
					return false
				}
			}
		}

		return true
	})

	return found
}

func validateStateFile(folder string) (string, error) {
	path := filepath.Join(folder, "state.go")
	info, err := os.Stat(path)
	if err != nil {
		return path, err
	}
	if info.IsDir() {
		return path, fmt.Errorf("state.go is a directory")
	}
	return path, nil
}

func parseStateFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse state.go: %v", err)
	}
	return fset, file, nil
}

func extractStateStruct(file *ast.File) (*ast.StructType, map[string]ast.Expr) {
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
	return stateStruct, typeDecls
}

func validateStateFields(
	fset *token.FileSet, stateStruct *ast.StructType, 
	typeDecls map[string]ast.Expr, path string,
) []lintIssue {
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

func checkState(folder string) []lintIssue {
	path, err := validateStateFile(folder)
	if err != nil {
		if os.IsNotExist(err) {
			return []lintIssue{issueAtPath(path, "Rule 2.1", "state.go file does not exist")}
		}
		return []lintIssue{issueAtPath(path, "Rule 2.1", err.Error())}
	}

	fset, file, err := parseStateFile(path)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 2.1", err.Error())}
	}

	stateStruct, typeDecls := extractStateStruct(file)
	if stateStruct == nil {
		return []lintIssue{newIssue(fset, file, path, "Rule 2.1", "`state` struct not found")}
	}

	return validateStateFields(fset, stateStruct, typeDecls, path)
}

func validateSpecFile(folder string) (string, error) {
	path := filepath.Join(folder, "spec.go")
	info, err := os.Stat(path)
	if err != nil {
		return path, err
	}
	if info.IsDir() {
		return path, fmt.Errorf("spec.go is a directory")
	}
	return path, nil
}

func parseSpecFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse spec.go: %v", err)
	}
	return fset, file, nil
}

func extractSpecTypes(
	file *ast.File,
) (map[string]ast.Expr, map[string]*ast.StructType, *ast.StructType, *ast.TypeSpec) {
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
	return typeDecls, suffixSpecs, specStruct, specTypeSpec
}

func validateDefaultsFunction(funcDecl *ast.FuncDecl, fset *token.FileSet, path string) []lintIssue {
	var issues []lintIssue
	if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
		issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.2", "defaults() must not take parameters"))
	}
	if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
		issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.2", "defaults() must return Spec"))
	} else if ident, ok := funcDecl.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "Spec" {
		message := "defaults() must return Spec"
		issues = append(issues, newIssue(fset, funcDecl.Type.Results.List[0].Type, path, "Rule 3.2", message))
	}
	return issues
}

func validateValidateMethod(funcDecl *ast.FuncDecl, fset *token.FileSet, path string) []lintIssue {
	var issues []lintIssue
	if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
		issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.4", "validate() must not take parameters"))
	}
	if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
		issues = append(issues, newIssue(fset, funcDecl, path, "Rule 3.4", "validate() must return error"))
	} else if ident, ok := funcDecl.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "error" {
		message := "validate() must return error"
		issues = append(issues, newIssue(fset, funcDecl.Type.Results.List[0].Type, path, "Rule 3.4", message))
	}
	return issues
}

func validateSpecFunctions(file *ast.File, fset *token.FileSet, path string) ([]lintIssue, bool, bool) {
	var issues []lintIssue
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
				issues = append(issues, validateDefaultsFunction(funcDecl, fset, path)...)
			}
			continue
		}
		
		recvType := receiverIdent(funcDecl.Recv.List[0].Type)
		if recvType == "Spec" && funcDecl.Name.Name == "validate" {
			validateFound = true
			issues = append(issues, validateValidateMethod(funcDecl, fset, path)...)
		}
	}
	
	return issues, defaultsFound, validateFound
}

func checkSpec(folder string) []lintIssue {
	path, err := validateSpecFile(folder)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []lintIssue{issueAtPath(path, "Rule 3.1", err.Error())}
	}

	fset, file, err := parseSpecFile(path)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 3.1", err.Error())}
	}

	typeDecls, suffixSpecs, specStruct, specTypeSpec := extractSpecTypes(file)

	var issues []lintIssue
	if specStruct == nil {
		issues = append(issues, issueAtPath(path, "Rule 3.1", "`Spec` struct not found"))
	} else {
		structIssues := collectStructIssues(specStruct, "Spec.", "Rule 3.2", fset, path, typeDecls)
		issues = append(issues, structIssues...)
	}

	funcIssues, defaultsFound, validateFound := validateSpecFunctions(file, fset, path)
	issues = append(issues, funcIssues...)

	if !defaultsFound {
		issues = append(issues, newIssue(fset, file.Name, path, "Rule 3.3", "defaults() function not found"))
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
		structIssues := collectStructIssues(structType, name+".", "Rule 3.2", fset, path, typeDecls)
		issues = append(issues, structIssues...)
	}

	return issues
}

func collectStructIssues(
	structType *ast.StructType, prefix, rule string, fset *token.FileSet, 
	path string, typeDecls map[string]ast.Expr,
) []lintIssue {
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

func handleIdentType(t *ast.Ident, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
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
}

func handleStructType(t *ast.StructType, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
	issues := make([]typeIssue, 0, len(t.Fields.List))
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
}

func handleArrayType(t *ast.ArrayType, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
	nested := collectTypeIssues(t.Elt, typeDecls, visiting)
	for i := range nested {
		nested[i].Message = fmt.Sprintf("array element %s", nested[i].Message)
	}
	return nested
}

func handleMapType(t *ast.MapType, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
	issues := make([]typeIssue, 0, 2) // Pre-allocate for key and value issues
	for _, issue := range collectTypeIssues(t.Key, typeDecls, visiting) {
		issue.Message = fmt.Sprintf("map key %s", issue.Message)
		issues = append(issues, issue)
	}
	for _, issue := range collectTypeIssues(t.Value, typeDecls, visiting) {
		issue.Message = fmt.Sprintf("map value %s", issue.Message)
		issues = append(issues, issue)
	}
	return issues
}

func collectTypeIssues(expr ast.Expr, typeDecls map[string]ast.Expr, visiting map[string]bool) []typeIssue {
	switch t := expr.(type) {
	case *ast.Ident:
		return handleIdentType(t, typeDecls, visiting)
	case *ast.StructType:
		return handleStructType(t, typeDecls, visiting)
	case *ast.ArrayType:
		return handleArrayType(t, typeDecls, visiting)
	case *ast.MapType:
		return handleMapType(t, typeDecls, visiting)
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
