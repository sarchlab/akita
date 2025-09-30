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
	return lintIssue{Rule: rule, Message: msg, Pos: token.Position{Filename: path, Line: 1, Column: 1}}
}

func parseGoFile(path string) (*token.FileSet, *ast.File, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, nil, err
	}
	return fset, file, nil
}

func collectTypeDecls(file *ast.File) map[string]ast.Expr {
	result := make(map[string]ast.Expr)
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				result[typeSpec.Name.Name] = typeSpec.Type
			}
		}
	}
	return result
}

func findStructType(file *ast.File, name string) (*ast.TypeSpec, *ast.StructType) {
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != name {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if ok {
				return typeSpec, structType
			}
			return typeSpec, nil
		}
	}
	return nil, nil
}

type builderContext struct {
	path            string
	fset            *token.FileSet
	file            *ast.File
	typeSpec        *ast.TypeSpec
	structType      *ast.StructType
	fieldTypes      map[string]ast.Expr
	withDecls       map[string]*ast.FuncDecl
	withAssignments map[string]map[string]bool
}

func newBuilderContext(folder string) (builderContext, []lintIssue) {
	path := filepath.Join(folder, "builder.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return builderContext{}, []lintIssue{issueAtPath(path, "Rule 4.1", "builder.go file does not exist")}
		}
		return builderContext{}, []lintIssue{issueAtPath(path, "Rule 4.1", err.Error())}
	}
	if info.IsDir() {
		return builderContext{}, []lintIssue{issueAtPath(path, "Rule 4.1", "builder.go is a directory")}
	}

	fset, file, err := parseGoFile(path)
	if err != nil {
		return builderContext{}, []lintIssue{issueAtPath(path, "Rule 4.1", fmt.Sprintf("failed to parse builder.go: %v", err))}
	}

	typeSpec, structType := findStructType(file, "Builder")
	if structType == nil {
		return builderContext{}, []lintIssue{newIssue(fset, file.Name, path, "Rule 4.1", "`Builder` struct not found")}
	}

	fieldTypes := make(map[string]ast.Expr)
	for _, field := range structType.Fields.List {
		for _, name := range field.Names {
			fieldTypes[name.Name] = field.Type
		}
	}

	withDecls, assignments := collectWithInfo(file)

	ctx := builderContext{
		path:            path,
		fset:            fset,
		file:            file,
		typeSpec:        typeSpec,
		structType:      structType,
		fieldTypes:      fieldTypes,
		withDecls:       withDecls,
		withAssignments: assignments,
	}
	return ctx, nil
}

func collectWithInfo(file *ast.File) (map[string]*ast.FuncDecl, map[string]map[string]bool) {
	decls := make(map[string]*ast.FuncDecl)
	assignments := make(map[string]map[string]bool)
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}
		if receiverIdent(funcDecl.Recv.List[0].Type) != "Builder" {
			continue
		}
		name := funcDecl.Name.Name
		if !strings.HasPrefix(name, "With") {
			continue
		}
		decls[name] = funcDecl
		assignments[name] = assignedBuilderFields(funcDecl)
	}
	return decls, assignments
}

func receiverName(funcDecl *ast.FuncDecl) string {
	if funcDecl.Recv == nil || len(funcDecl.Recv.List) != 1 {
		return ""
	}
	recv := funcDecl.Recv.List[0]
	if len(recv.Names) == 0 {
		return ""
	}
	return recv.Names[0].Name
}

func assignedBuilderFields(funcDecl *ast.FuncDecl) map[string]bool {
	result := make(map[string]bool)
	receiver := receiverName(funcDecl)
	if funcDecl.Body == nil || receiver == "" {
		return result
	}
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, lhs := range assign.Lhs {
			if sel, ok := lhs.(*ast.SelectorExpr); ok {
				if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == receiver {
					result[sel.Sel.Name] = true
				}
			}
		}
		return true
	})
	return result
}

func ensureLegacyBuilderFields(ctx builderContext) []lintIssue {
	required := []string{"Freq", "Engine"}
	var missing []string
	for _, name := range required {
		if _, ok := ctx.fieldTypes[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	msg := fmt.Sprintf("`Builder` struct must include fields %s", strings.Join(missing, ", "))
	return []lintIssue{newIssue(ctx.fset, ctx.typeSpec, ctx.path, "Rule 4.1", msg)}
}

func ensureWithSetters(ctx builderContext) []lintIssue {
	configured := make(map[string]bool)
	for name := range ctx.fieldTypes {
		if name == "spec" || name == "simulation" {
			continue
		}
		configured[name] = false
	}

	var issues []lintIssue
	for name, decl := range ctx.withDecls {
		if !returnsBuilderValue(decl) {
			msg := fmt.Sprintf("`%s` must return Builder", name)
			issues = append(issues, newIssue(ctx.fset, decl, ctx.path, "Rule 4.4", msg))
		}
		for field := range ctx.withAssignments[name] {
			if _, ok := configured[field]; ok {
				configured[field] = true
			}
		}
	}

	var missing []string
	for field, ok := range configured {
		if !ok {
			missing = append(missing, field)
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		msg := fmt.Sprintf("missing `With` setter for field(s): %s", strings.Join(missing, ", "))
		issues = append(issues, newIssue(ctx.fset, ctx.structType, ctx.path, "Rule 4.4", msg))
	}

	return issues
}

func ensureSimulationRules(ctx builderContext) []lintIssue {
	field, ok := ctx.fieldTypes["simulation"]
	var issues []lintIssue
	if !ok {
		issues = append(issues, newIssue(ctx.fset, ctx.typeSpec, ctx.path, "Rule 4.2", "`Builder` struct must include `simulation` field"))
	} else if !isPointerTo(field, "simv5", "Simulation") {
		issues = append(issues, newIssue(ctx.fset, field, ctx.path, "Rule 4.2", "`simulation` field must be of type *simv5.Simulation"))
	}

	decl, ok := ctx.withDecls["WithSimulation"]
	if !ok {
		issues = append(issues, newIssue(ctx.fset, ctx.typeSpec, ctx.path, "Rule 4.2", "`WithSimulation` method not found"))
		return issues
	}

	if !ctx.withAssignments["WithSimulation"]["simulation"] {
		issues = append(issues, newIssue(ctx.fset, decl, ctx.path, "Rule 4.2", "`WithSimulation` must assign to the `simulation` field"))
	}

	if decl.Type.Params == nil || decl.Type.Params.NumFields() != 1 {
		issues = append(issues, newIssue(ctx.fset, decl, ctx.path, "Rule 4.2", "`WithSimulation` must take exactly one simv5.Simulation argument"))
	} else {
		paramType := decl.Type.Params.List[0].Type
		if !isSelector(paramType, "simv5", "Simulation") {
			issues = append(issues, newIssue(ctx.fset, paramType, ctx.path, "Rule 4.2", "`WithSimulation` argument must be simv5.Simulation"))
		}
	}

	return issues
}

func ensureSpecRules(ctx builderContext) []lintIssue {
	field, ok := ctx.fieldTypes["spec"]
	var issues []lintIssue
	if !ok {
		issues = append(issues, newIssue(ctx.fset, ctx.typeSpec, ctx.path, "Rule 4.3", "`Builder` struct must include `spec` field"))
	} else if ident, ok := field.(*ast.Ident); !ok || ident.Name != "Spec" {
		issues = append(issues, newIssue(ctx.fset, field, ctx.path, "Rule 4.3", "`spec` field must use Spec type"))
	}

	decl, ok := ctx.withDecls["WithSpec"]
	if !ok {
		issues = append(issues, newIssue(ctx.fset, ctx.typeSpec, ctx.path, "Rule 4.3", "`WithSpec` method not found"))
		return issues
	}

	if !ctx.withAssignments["WithSpec"]["spec"] {
		issues = append(issues, newIssue(ctx.fset, decl, ctx.path, "Rule 4.3", "`WithSpec` must assign to the `spec` field"))
	}
	if decl.Type.Params == nil || decl.Type.Params.NumFields() != 1 {
		issues = append(issues, newIssue(ctx.fset, decl, ctx.path, "Rule 4.3", "`WithSpec` must take exactly one Spec argument"))
	} else {
		paramType := decl.Type.Params.List[0].Type
		if ident, ok := paramType.(*ast.Ident); !ok || ident.Name != "Spec" {
			issues = append(issues, newIssue(ctx.fset, paramType, ctx.path, "Rule 4.3", "`WithSpec` argument must be Spec"))
		}
	}

	return issues
}

func ensureBuildMethod(ctx builderContext) []lintIssue {
	buildDecl := findBuildFunc(ctx.file)
	if buildDecl == nil {
		return []lintIssue{newIssue(ctx.fset, ctx.file, ctx.path, "Rule 4.5", "`Build` method not found")}
	}

	var issues []lintIssue
	if params := buildDecl.Type.Params; params == nil || params.NumFields() != 1 {
		issues = append(issues, newIssue(ctx.fset, buildDecl, ctx.path, "Rule 4.5", "`Build` must take exactly one argument"))
	} else {
		paramType := params.List[0].Type
		if ident, ok := paramType.(*ast.Ident); !ok || ident.Name != "string" {
			issues = append(issues, newIssue(ctx.fset, paramType, ctx.path, "Rule 4.5", "`Build` argument must be of type string"))
		}
	}

	if results := buildDecl.Type.Results; results == nil || results.NumFields() != 1 {
		issues = append(issues, newIssue(ctx.fset, buildDecl, ctx.path, "Rule 4.5", "`Build` must return *Comp"))
	} else {
		resType := results.List[0].Type
		if !isPointerTo(resType, "", "Comp") {
			issues = append(issues, newIssue(ctx.fset, resType, ctx.path, "Rule 4.5", "`Build` must return *Comp"))
		}
	}

	if !buildCallsSpecValidate(buildDecl) {
		issues = append(issues, newIssue(ctx.fset, buildDecl, ctx.path, "Rule 4.6", "`Build` must validate the spec"))
	}

	return issues
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
	ctx, errs := newBuilderContext(folder)
	if len(errs) > 0 {
		return errs
	}

	var issues []lintIssue
	issues = append(issues, ensureLegacyBuilderFields(ctx)...)
	issues = append(issues, ensureWithSetters(ctx)...)
	issues = append(issues, ensureSimulationRules(ctx)...)
	issues = append(issues, ensureSpecRules(ctx)...)
	issues = append(issues, ensureBuildMethod(ctx)...)
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

func validateSimulationRequirements(fset *token.FileSet, path string, builderSpec *ast.TypeSpec, field ast.Expr, withDecl *ast.FuncDecl, assignments map[string]bool) []lintIssue {
	var issues []lintIssue
	if field == nil {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.2", "`Builder` struct must include `simulation` field"))
		return issues
	}

	typeStr := exprString(fset, field)
	if typeStr != "*simv5.Simulation" && typeStr != "simv5.Simulation" {
		issues = append(issues, newIssue(fset, field, path, "Rule 4.2", "`simulation` field must be of type *simv5.Simulation"))
	}

	if withDecl == nil {
		issues = append(issues, newIssue(fset, builderSpec, path, "Rule 4.2", "`WithSimulation` method not found"))
		return issues
	}

	if assignments == nil || !assignments["simulation"] {
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.2", "`WithSimulation` must assign to the `simulation` field"))
	}

	if withDecl.Type.Params == nil || withDecl.Type.Params.NumFields() != 1 {
		issues = append(issues, newIssue(fset, withDecl, path, "Rule 4.2", "`WithSimulation` must take exactly one simv5.Simulation argument"))
	} else {
		paramType := exprString(fset, withDecl.Type.Params.List[0].Type)
		if paramType != "*simv5.Simulation" && paramType != "simv5.Simulation" {
			issues = append(issues, newIssue(fset, withDecl.Type.Params.List[0].Type, path, "Rule 4.2", "`WithSimulation` argument must be simv5.Simulation"))
		}
	}

	return issues
}

func validateSpecRequirements(fset *token.FileSet, path string, builderSpec *ast.TypeSpec, field ast.Expr, withDecl *ast.FuncDecl, assignments map[string]bool) []lintIssue {
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
			issues = append(issues, newIssue(fset, withDecl.Type.Params.List[0].Type, path, "Rule 4.3", "`WithSpec` argument must be Spec"))
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

func isSelector(expr ast.Expr, pkg, name string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	if name != "" && sel.Sel.Name != name {
		return false
	}
	ident, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	if pkg != "" && ident.Name != pkg {
		return false
	}
	return true
}

func isPointerTo(expr ast.Expr, pkg, name string) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	if ident, ok := star.X.(*ast.Ident); ok {
		return pkg == "" && ident.Name == name
	}
	return isSelector(star.X, pkg, name)
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

func buildCallsSpecValidate(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Body == nil {
		return false
	}
	receiver := receiverName(funcDecl)
	aliases := collectSpecAliases(funcDecl.Body, receiver)
	return specValidateCallExists(funcDecl.Body, receiver, aliases)
}

func collectSpecAliases(body *ast.BlockStmt, receiver string) map[string]bool {
	aliases := make(map[string]bool)
	if receiver == "" {
		return aliases
	}
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range assign.Lhs {
			ident, ok := lhs.(*ast.Ident)
			if !ok || i >= len(assign.Rhs) {
				continue
			}
			if refersToSpec(assign.Rhs[i], receiver, aliases) {
				aliases[ident.Name] = true
			}
		}
		return true
	})
	return aliases
}

func specValidateCallExists(body *ast.BlockStmt, receiver string, aliases map[string]bool) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel == nil {
			return true
		}
		if sel.Sel.Name != "validate" && sel.Sel.Name != "Validate" {
			return true
		}
		if refersToSpec(sel.X, receiver, aliases) {
			found = true
			return false
		}
		return true
	})
	return found
}

func refersToSpec(expr ast.Expr, receiver string, aliases map[string]bool) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if ok {
		if ident, ok := sel.X.(*ast.Ident); ok && ident.Name == receiver && sel.Sel.Name == "spec" {
			return true
		}
		if ident, ok := sel.X.(*ast.Ident); ok && aliases[ident.Name] {
			return true
		}
	}
	ident, ok := expr.(*ast.Ident)
	return ok && aliases[ident.Name]
}

func checkState(folder string) []lintIssue {
	path := filepath.Join(folder, "state.go")
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []lintIssue{issueAtPath(path, "Rule 2.1", "state.go file does not exist")}
		}
		return []lintIssue{issueAtPath(path, "Rule 2.1", err.Error())}
	}
	if info.IsDir() {
		return []lintIssue{issueAtPath(path, "Rule 2.1", "state.go is a directory")}
	}

	fset, file, err := parseGoFile(path)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 2.1", fmt.Sprintf("failed to parse state.go: %v", err))}
	}

	typeDecls := collectTypeDecls(file)
	_, stateStruct := findStructType(file, "state")
	if stateStruct == nil {
		return []lintIssue{newIssue(fset, file, path, "Rule 2.1", "`state` struct not found")}
	}

	return collectStructIssues(stateStruct, "state.", "Rule 2.1", fset, path, typeDecls)
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

	fset, file, err := parseGoFile(path)
	if err != nil {
		return []lintIssue{issueAtPath(path, "Rule 3.1", fmt.Sprintf("failed to parse spec.go: %v", err))}
	}

	typeDecls := collectTypeDecls(file)
	specType, specStruct := findStructType(file, "Spec")

	var issues []lintIssue
	if specStruct == nil {
		issues = append(issues, newIssue(fset, file, path, "Rule 3.1", "`Spec` struct not found"))
	} else {
		issues = append(issues, collectStructIssues(specStruct, "Spec.", "Rule 3.4", fset, path, typeDecls)...)
	}

	_, defaultsIssues := analyzeDefaultsFunc(file, fset, path)
	issues = append(issues, defaultsIssues...)
	_, validateIssues := analyzeValidateMethod(file, specType, fset, path)
	issues = append(issues, validateIssues...)

	for name, expr := range typeDecls {
		if name == "Spec" || !strings.HasSuffix(name, "Spec") {
			continue
		}
		structType, ok := expr.(*ast.StructType)
		if !ok {
			msg := fmt.Sprintf("%s must be a struct", name)
			issues = append(issues, newIssue(fset, file, path, "Rule 3.4", msg))
			continue
		}
		issues = append(issues, collectStructIssues(structType, name+".", "Rule 3.4", fset, path, typeDecls)...)
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
