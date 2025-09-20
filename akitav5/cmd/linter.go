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
	"strings"
)

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
		fmt.Printf("\tmarker scan failed: %v\n", markerErr)
		return true
	}
	if !hasMarker {
		fmt.Println("\t-- not a component")
		return false
	}

	var errs []string

	if errCompStruct := checkComponentFormat(folderPath); errCompStruct != nil {
		errs = append(errs, fmt.Sprintf("Rule 1.3: %s", errCompStruct))
	}

	if errBuilder := checkBuilderFileExistence(folderPath); errBuilder != nil {
		errs = append(errs, fmt.Sprintf("Rule 5.1: %s", errBuilder))
	} else {
		node, errParseBuilder := ParseBuilderFile(folderPath)
		if errParseBuilder != nil {
			errs = append(errs, fmt.Sprintf("Rule 5.1: %s", errParseBuilder))
		} else {
			if errBuilderStruct := checkBuilderStruct(node); errBuilderStruct != nil {
				errs = append(errs, fmt.Sprintf("Rule 5.1: %s", errBuilderStruct))
			}
			if errBuilderParameter := checkBuilderParameters(node); errBuilderParameter != nil {
				errs = append(errs, fmt.Sprintf("Rule 5.2: %s", errBuilderParameter))
			}
			if errWithFunc := checkWithFunc(node); errWithFunc != nil {
				errs = append(errs, fmt.Sprintf("Rule 5.3: %s", errWithFunc))
			}
			if errWithReturn := checkWithFuncReturn(node); errWithReturn != nil {
				errs = append(errs, fmt.Sprintf("Rule 5.4: %s", errWithReturn))
			}
			if errBuilderFunc := checkBuildFunction(node); errBuilderFunc != nil {
				errs = append(errs, fmt.Sprintf("Rule 5.5: %s", errBuilderFunc))
			} else {
				if errParam := checkBuildFunctionParam(node); errParam != nil {
					errs = append(errs, fmt.Sprintf("Rule 5.6: %s", errParam))
				}
				if errReturn := checkBuildFunctionReturn(node); errReturn != nil {
					errs = append(errs, fmt.Sprintf("Rule 5.7: %s", errReturn))
				}
			}
		}
	}

	if stateErrs := checkStatePurity(folderPath); len(stateErrs) > 0 {
		errs = append(errs, stateErrs...)
	}

	if specErrs := checkSpecRules(folderPath); len(specErrs) > 0 {
		errs = append(errs, specErrs...)
	}

	if len(errs) == 0 {
		fmt.Println("\tOK")
		return false
	}
	for _, errMsg := range errs {
		fmt.Printf("\t%s\n", errMsg)
	}
	return true
}

var builtinTypes = map[string]struct{}{
	"bool": {}, "byte": {}, "complex64": {}, "complex128": {}, "error": {}, "float32": {}, "float64": {},
	"int": {}, "int8": {}, "int16": {}, "int32": {}, "int64": {}, "rune": {}, "string": {},
	"uint": {}, "uint8": {}, "uint16": {}, "uint32": {}, "uint64": {}, "uintptr": {}, "any": {},
}

func checkComponentFormat(folderPath string) error {
	compFilePath := filepath.Join(folderPath, "comp.go")
	if _, err := os.Stat(compFilePath); os.IsNotExist(err) {
		return fmt.Errorf("comp.go file does not exist")
	}

	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, compFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse comp.go file %s: %v", compFilePath, err)
	}

	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if ok && typeSpec.Name.Name == "Comp" {
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("no Comp struct in comp.go")
}

func checkBuilderFileExistence(folderPath string) error {
	// check builder.go existence
	builderFilePath := filepath.Join(folderPath, "builder.go")
	if _, err := os.Stat(builderFilePath); os.IsNotExist(err) {
		return fmt.Errorf("builder.go file does not exist")
	}
	return nil
}

func ParseBuilderFile(folderPath string) (*ast.File, error) {
	builderFilePath := filepath.Join(folderPath, "builder.go")

	// parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to parse builder.go file %s: %v",
			builderFilePath, err)
	}
	return node, nil
}

func checkBuilderStruct(node *ast.File) error {
	existBuilderStruct := false
	for _, decl := range node.Decls { // iterate all declaration
		genDecl, ok := decl.(*ast.GenDecl)    // check if decl is one of GenDecl
		if !ok || genDecl.Tok != token.TYPE { // check if decl is a type decl
			continue
		}
		for _, spec := range genDecl.Specs { // iterate specs in the type decl
			typeSpec, ok := spec.(*ast.TypeSpec)       //check if spec in Expr
			if ok && typeSpec.Name.Name == "Builder" { // check struct name
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					existBuilderStruct = true
					break
				}
			}
		}
	}
	if !existBuilderStruct {
		return fmt.Errorf("no Builder struct in builder.go")
	}

	return nil
}

// Logic: if the builder field has a setter statement in a `With` function,
// or has no setter at all, pass;
// if it has a setter statement but located in a func not named by `With...`,
// return error.
func checkWithFunc(node *ast.File) error {
	builderFields, configurableFields := getBuilderFields(node)

	// find the object of all configuration func
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}
		getConfigurableFields(builderFields, configurableFields, funcDecl)
	}

	var unconfigs []string
	for key, value := range configurableFields {
		if !value {
			unconfigs = append(unconfigs, key)
		}
	}

	if len(unconfigs) != 0 {
		unconfig := strings.Join(unconfigs, ", ")
		return fmt.Errorf("configurable parameter(s) [%s] missing "+
			"proper setter function(s) starting with 'With'", unconfig)
	}

	return nil
}

func getBuilderFields(node *ast.File) (map[string]bool, map[string]bool) {
	builderFields := map[string]bool{}
	configurableFields := map[string]bool{}

	// find all fields in Builder struct
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok || typeSpec.Name.Name != "Builder" {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				for _, fieldName := range field.Names {
					// Assume all parameters are configurable
					builderFields[fieldName.Name] = true
					configurableFields[fieldName.Name] = false
				}
			}
		}
	}

	return builderFields, configurableFields
}

func getConfigurableFields(builderFields map[string]bool,
	configurableFields map[string]bool, funcDecl *ast.FuncDecl) {
	receiverName := getRecieverName(funcDecl)

	// find assignment receiver.<field> = ...
	ast.Inspect(funcDecl.Body, func(n ast.Node) bool { // iterate statements
		assign, ok := n.(*ast.AssignStmt) // check if stmt is an assignment
		if !ok {
			return true // continue to iterate every subnode of the node
		}
		for _, lhs := range assign.Lhs {
			// if left is a selector expression
			if sel, ok := lhs.(*ast.SelectorExpr); ok {
				ident, ok := sel.X.(*ast.Ident)
				if ok && ident.Name == receiverName {
					fieldName := sel.Sel.Name
					if builderFields[fieldName] && strings.HasPrefix(
						funcDecl.Name.Name, "With") {
						configurableFields[fieldName] = true
						// changes the original configurableFields
					}
				}
			}
		}
		return true
	})
}

func getRecieverName(funcDecl *ast.FuncDecl) string {
	// record receiver name
	receiverName := ""
	if funcDecl.Recv != nil && len(funcDecl.Recv.List) == 1 {
		if len(funcDecl.Recv.List[0].Names) == 1 {
			receiverName = funcDecl.Recv.List[0].Names[0].Name
		}
	}
	return receiverName
}

func checkWithFuncReturn(node *ast.File) error {
	var improperReturns []string
	for _, decl := range node.Decls { // iterate all declaration
		funcDecl, ok := decl.(*ast.FuncDecl) // check if decl is a FuncDecl
		if !ok {
			continue
		}
		if isImproperWithFunction(funcDecl) {
			improperReturns = append(improperReturns, funcDecl.Name.Name)
		}
	}
	if len(improperReturns) != 0 {
		funcList := strings.Join(improperReturns, ", ")
		return fmt.Errorf("'With' function(s) [%s] not returning "+
			"builder type value", funcList)
	}

	return nil
}

// checks if name of the func decl is improper
func isImproperWithFunction(funcDecl *ast.FuncDecl) bool {
	if funcDecl.Recv == nil || funcDecl.Name == nil {
		return false
	}
	if !strings.HasPrefix(funcDecl.Name.Name, "With") {
		return false
	}
	if funcDecl.Type.Results == nil {
		return false
	}
	for _, result := range funcDecl.Type.Results.List {
		ident, ok := result.Type.(*ast.Ident)
		if !ok || ident.Name != "Builder" {
			return true
		}
	}
	return false
}

func checkBuilderParameters(node *ast.File) error {
	parameters := []string{}
	mustInclude := 0
	isBuilderStruct := false

	// Find all field in Builder struct
	for _, decl := range node.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.TYPE {
			continue
		}
		parameters, mustInclude, isBuilderStruct = countBuilderFields(genDecl)
		if isBuilderStruct {
			break // since there can only be one Builder Struct
		}
	}

	if len(parameters) < 2 || mustInclude != 2 {
		return fmt.Errorf("builder must include at least 2 parameters, " +
			"including Freq and Engine")
	}

	return nil
}

func countBuilderFields(genDecl *ast.GenDecl) ([]string, int, bool) {
	parameters := []string{}
	mustInclude := 0
	isBuilderStruct := false

	// Find all field in a struct named Builder
	for _, spec := range genDecl.Specs {
		typeSpec, ok := spec.(*ast.TypeSpec)
		if !ok || typeSpec.Name.Name != "Builder" {
			continue
		}
		structType, ok := typeSpec.Type.(*ast.StructType)
		if !ok {
			continue
		}
		isBuilderStruct = true
		for _, field := range structType.Fields.List {
			for _, fieldName := range field.Names {
				if fieldName.Name == "Freq" || fieldName.Name == "Engine" {
					mustInclude += 1
				}
				parameters = append(parameters, fieldName.Name)
			}
		}
	}

	return parameters, mustInclude, isBuilderStruct
}

func checkBuildFunction(node *ast.File) error {
	found := false
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Build" {
			continue
		}
		found = true
	}

	if !found {
		return fmt.Errorf("`Build` function not found in builder")
	}

	return nil
}

func checkBuildFunctionParam(node *ast.File) error {
	if err := getBuildFunctionParamErr(node); err != nil {
		return err
	}

	return nil
}

func checkBuildFunctionReturn(node *ast.File) error {
	if err := getBuildFunctionReturnErr(node); err != nil {
		return err
	}

	return nil
}

func getBuildFunctionParamErr(node *ast.File) error {
	// Check if func has exactly one parameter
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Build" {
			continue
		}
		// Check if build takes only one parameter
		if funcDecl.Type.Params.NumFields() != 1 {
			return fmt.Errorf("`Build` function must take exactly one argument")
		}

		// Check if the parameter type is string
		param := funcDecl.Type.Params.List[0]
		ident, ok := param.Type.(*ast.Ident)
		if !ok || ident.Name != "string" {
			return fmt.Errorf("`Build` function takes only string as argument")
		}
	}
	return nil
}

func getBuildFunctionReturnErr(node *ast.File) error {
	// Check if func has exactly one parameter
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Build" {
			continue
		}

		// Check if the return type is a pointer
		if funcDecl.Type.Results == nil || len(funcDecl.Type.Results.List) == 0 {
			return fmt.Errorf("`Build` function must have a return value")
		}
		retType := funcDecl.Type.Results.List[0].Type
		_, ok = retType.(*ast.StarExpr)
		if !ok {
			return fmt.Errorf("`Build` function must return pointer type")
		}

		// check if the return type is Comp
		retIdent, ok := retType.(*ast.StarExpr).X.(*ast.Ident)
		if !ok || retIdent.Name != "Comp" {
			return fmt.Errorf("`Build` function must return pointer to Comp")
		}
	}
	return nil
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

func checkStatePurity(folderPath string) []string {
	statePath := filepath.Join(folderPath, "state.go")
	info, err := os.Stat(statePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []string{fmt.Sprintf("Rule 2.1: failed to read state.go: %v", err)}
	}
	if info.IsDir() {
		return []string{fmt.Sprintf("Rule 2.1: expected file state.go, found directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, statePath, nil, 0)
	if err != nil {
		return []string{fmt.Sprintf("Rule 2.1: failed to parse state.go: %v", err)}
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

	var violations []string
	for _, field := range stateStruct.Fields.List {
		if field == nil {
			continue
		}
		if violation := pureFieldViolation(field.Type, typeDecls, map[string]bool{}, fset); violation != "" {
			if len(field.Names) == 0 {
				violations = append(violations, fmt.Sprintf("Rule 2.1: state.%s %s", exprString(fset, field.Type), violation))
				continue
			}
			for _, name := range field.Names {
				violations = append(violations, fmt.Sprintf("Rule 2.1: state.%s %s", name.Name, violation))
			}
		}
	}

	return violations
}

func pureFieldViolation(expr ast.Expr, typeDecls map[string]ast.Expr, visiting map[string]bool, fset *token.FileSet) string {
	switch t := expr.(type) {
	case *ast.Ident:
		if _, ok := builtinTypes[t.Name]; ok {
			return ""
		}
		if decl, ok := typeDecls[t.Name]; ok {
			if visiting[t.Name] {
				return ""
			}
			visiting[t.Name] = true
			violation := pureFieldViolation(decl, typeDecls, visiting, fset)
			delete(visiting, t.Name)
			if violation != "" {
				if _, ok := decl.(*ast.StructType); ok {
					return fmt.Sprintf("contains non-pure data in %s: %s", t.Name, violation)
				}
				return violation
			}
			return ""
		}
		return fmt.Sprintf("uses external type %s", t.Name)
	case *ast.StructType:
		for _, field := range t.Fields.List {
			violation := pureFieldViolation(field.Type, typeDecls, visiting, fset)
			if violation != "" {
				fieldName := "embedded"
				if len(field.Names) > 0 {
					fieldName = field.Names[0].Name
				} else {
					fieldName = exprString(fset, field.Type)
				}
				return fmt.Sprintf("field %s %s", fieldName, violation)
			}
		}
		return ""
	case *ast.ArrayType:
		return pureFieldViolation(t.Elt, typeDecls, visiting, fset)
	case *ast.ParenExpr:
		return pureFieldViolation(t.X, typeDecls, visiting, fset)
	case *ast.StarExpr:
		return fmt.Sprintf("contains pointer type %s", exprString(fset, t))
	case *ast.ChanType:
		return "contains channel type"
	case *ast.FuncType:
		return "contains function type"
	case *ast.MapType:
		if keyViolation := pureFieldViolation(t.Key, typeDecls, visiting, fset); keyViolation != "" {
			return fmt.Sprintf("contains map key type violating purity: %s", keyViolation)
		}
		if valViolation := pureFieldViolation(t.Value, typeDecls, visiting, fset); valViolation != "" {
			return fmt.Sprintf("contains map value type violating purity: %s", valViolation)
		}
		return ""
	case *ast.InterfaceType:
		return "contains interface type"
	case *ast.SelectorExpr:
		return ""
	case *ast.IndexExpr:
		return "contains generic type expression"
	case *ast.IndexListExpr:
		return "contains generic type expression"
	default:
		return fmt.Sprintf("uses unsupported type %T", t)
	}
}

func exprString(fset *token.FileSet, expr ast.Expr) string {
	var buf bytes.Buffer
	if err := printer.Fprint(&buf, fset, expr); err != nil {
		return ""
	}
	return buf.String()
}

func checkSpecRules(folderPath string) []string {
	specPath := filepath.Join(folderPath, "spec.go")
	info, err := os.Stat(specPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return []string{fmt.Sprintf("Rule 3.1: failed to read spec.go: %v", err)}
	}
	if info.IsDir() {
		return []string{fmt.Sprintf("Rule 3.1: expected file spec.go, found directory")}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, specPath, nil, 0)
	if err != nil {
		return []string{fmt.Sprintf("Rule 3.1: failed to parse spec.go: %v", err)}
	}

	typeDecls := map[string]ast.Expr{}
	var specStruct *ast.StructType
	suffixSpecTypes := map[string]ast.Expr{}

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
				suffixSpecTypes[typeSpec.Name.Name] = typeSpec.Type
			}
			if typeSpec.Name.Name == "Spec" {
				if structType, ok := typeSpec.Type.(*ast.StructType); ok {
					specStruct = structType
				}
			}
		}
	}

	var violations []string
	if specStruct == nil {
		violations = append(violations, "Rule 3.1: spec.go must define type Spec struct")
	} else {
		for _, field := range specStruct.Fields.List {
			if field == nil {
				continue
			}
			if violation := pureFieldViolation(field.Type, typeDecls, map[string]bool{}, fset); violation != "" {
				if len(field.Names) == 0 {
					violations = append(violations, fmt.Sprintf("Rule 3.1: Spec.%s %s", exprString(fset, field.Type), violation))
					continue
				}
				for _, name := range field.Names {
					violations = append(violations, fmt.Sprintf("Rule 3.1: Spec.%s %s", name.Name, violation))
				}
			}
		}
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
				if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
					violations = append(violations, "Rule 3.2: defaults() must not take parameters")
					continue
				}
				if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
					violations = append(violations, "Rule 3.2: defaults() must return Spec")
					continue
				}
				result := funcDecl.Type.Results.List[0].Type
				if ident, ok := result.(*ast.Ident); !ok || ident.Name != "Spec" {
					violations = append(violations, "Rule 3.2: defaults() must return Spec")
					continue
				}
				defaultsFound = true
			}
			continue
		}
		recv := funcDecl.Recv.List[0].Type
		var recvType *ast.Ident
		switch r := recv.(type) {
		case *ast.Ident:
			recvType = r
		case *ast.StarExpr:
			if ident, ok := r.X.(*ast.Ident); ok {
				recvType = ident
			}
		}
		if recvType != nil && recvType.Name == "Spec" && funcDecl.Name.Name == "validate" {
			if funcDecl.Type.Params != nil && funcDecl.Type.Params.NumFields() != 0 {
				violations = append(violations, "Rule 3.3: validate() must not take parameters")
				continue
			}
			if funcDecl.Type.Results == nil || funcDecl.Type.Results.NumFields() != 1 {
				violations = append(violations, "Rule 3.3: validate() must return error")
				continue
			}
			if ident, ok := funcDecl.Type.Results.List[0].Type.(*ast.Ident); !ok || ident.Name != "error" {
				violations = append(violations, "Rule 3.3: validate() must return error")
				continue
			}
			validateFound = true
		}
	}

	if !defaultsFound {
		violations = append(violations, "Rule 3.2: defaults() function not found")
	}
	if !validateFound {
		violations = append(violations, "Rule 3.3: validate() method on Spec not found")
	}

	for name, expr := range suffixSpecTypes {
		structType, ok := expr.(*ast.StructType)
		if !ok {
			violations = append(violations, fmt.Sprintf("Rule 3.4: %s must be a struct", name))
			continue
		}
		for _, field := range structType.Fields.List {
			if field == nil {
				continue
			}
			if violation := pureFieldViolation(field.Type, typeDecls, map[string]bool{}, fset); violation != "" {
				fieldName := "embedded"
				if len(field.Names) > 0 {
					fieldName = field.Names[0].Name
				} else {
					fieldName = exprString(fset, field.Type)
				}
				violations = append(violations, fmt.Sprintf("Rule 3.4: %s.%s %s", name, fieldName, violation))
			}
		}
	}

	return violations
}
