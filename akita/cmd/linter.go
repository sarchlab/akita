package cmd

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

var linterCmd = &cobra.Command{
	Use:   "check",
	Short: "Check component format.",
	Long:  "`check [component folder path]` checks the component format.",
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) < 1 {
			fmt.Fprintln(os.Stderr, "Error: component folder path argument is required")
			os.Exit(1)
		}
		folderPath := args[0]

		hasError := false
		errCompStruct := checkComponentFormat(folderPath)
		if errCompStruct != nil {
			fmt.Print(errCompStruct, "\n")
			hasError = true
		}

		errBuilder := checkBuilderFileExistence(folderPath)
		if errBuilder != nil {
			fmt.Printf("<2> Builder error: %s\n", errBuilder)
			hasError = true
		} else {
			node, errParseBuilder := ParseBuilderFile(folderPath)
			if errParseBuilder != nil {
				fmt.Printf("<2> Builder parse error: %s\n", errParseBuilder)
				hasError = true
			} else {
				errBuilderStruct := checkBuilderStruct(node)
				if errBuilderStruct != nil {
					fmt.Printf("<2a> Builder structure error: %s\n",
						errBuilderStruct)
					hasError = true
				}

				errWithFunc := checkWithFunc(node)
				if errWithFunc != nil {
					fmt.Printf("<2b> Builder format error: %s\n", errWithFunc)
					hasError = true
				}

				errWithReturn := checkWithFuncReturn(node)
				if errWithReturn != nil {
					fmt.Printf("<2b> Builder return error: %s\n", errWithReturn)
					hasError = true
				}

				errBuilderParameter := checkBuilderParameters(node)
				if errBuilderParameter != nil {
					fmt.Printf("<2c> Builder parameter error: %s\n",
						errBuilderParameter)
					hasError = true
				}

				errBuilderFunc := checkBuildFunction(node)
				if errBuilderFunc != nil {
					fmt.Printf("<2d> Builder function error: %s\n",
						errBuilderFunc)
					hasError = true
				} else {
					errParam := checkBuildFunctionParam(node)
					if errParam != nil {
						fmt.Printf("<2d> Builder function error: %s\n",
							errParam)
						hasError = true
					}
					errReturn := checkBuildFunctionReturn(node)
					if errReturn != nil {
						fmt.Printf("<2d> Builder function error: %s\n",
							errReturn)
						hasError = true
					}
				}
			}
		}

		manifest, errManifest := checkManifestFile(folderPath)
		if errManifest != nil {
			fmt.Printf("<3> Manifest error: %v\n", errManifest)
			hasError = true
		} else {
			errManifestName := checkManifestName(manifest)
			if errManifestName != nil {
				fmt.Printf("<3a> Manifest name error: %s\n",
					errManifestName)
				hasError = true
			}
			errManifestPort := checkManifestPort(manifest)
			if errManifestPort != nil {
				fmt.Printf("<3b> Manifest port error: %s\n",
					errManifestPort)
				hasError = true
			}
			errManifestParam := checkManifestParam(manifest)
			if errManifestParam != nil {
				fmt.Printf("<3b> Manifest parameter error: %s\n",
					errManifestParam)
				hasError = true
			}
		}

		if hasError {
			os.Exit(1)
		} else {
			os.Exit(0)
		}

	},
}

func init() {
	rootCmd.AddCommand(linterCmd)
}

func checkComponentFormat(folderPath string) error {
	// check comp.go existence
	compFilePath := filepath.Join(folderPath, "comp.go")
	if _, err := os.Stat(compFilePath); os.IsNotExist(err) {
		return fmt.Errorf("<1> Component error: comp.go file does not exist")
	}

	// parse the comp file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, compFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("<1> Component error: failed to parse comp.go "+
			"file %s: %v", compFilePath, err)
	}

	for _, decl := range node.Decls { // iterate all declaration
		genDecl, ok := decl.(*ast.GenDecl)    // check if decl is in GenDecl
		if !ok || genDecl.Tok != token.TYPE { // check if decl is a type decl
			continue
		}
		for _, spec := range genDecl.Specs { // iterate specs in the type decl
			typeSpec, ok := spec.(*ast.TypeSpec)    //check if spec is in Expr
			if ok && typeSpec.Name.Name == "Comp" { // check struct name
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					return nil
				}
			}
		}
	}
	return fmt.Errorf("<1a> Component structure error: " +
		"no Comp struct in comp.go")
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
	receiverName := getReceiverName(funcDecl)

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

func getReceiverName(funcDecl *ast.FuncDecl) string {
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

func checkManifestFile(folderPath string) (map[string]any, error) {
	// Check manifest.json existence
	jsonFilePath := filepath.Join(folderPath, "manifest.json")
	if _, err := os.Stat(jsonFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("manifest.json file does not exist")
	}

	// Read the json file
	fileContent, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest.json: %v", err)
	}

	// Parse the json file
	var manifest map[string]any
	if err := json.Unmarshal(fileContent, &manifest); err != nil {
		return nil, fmt.Errorf("failed to parse manifest.json: %v", err)
	}

	return manifest, nil
}

func checkManifestName(manifest map[string]any) error {
	// Must have `name` attribute with a non-empty string value
	nameAtt, ok := manifest["name"].(string)
	if !ok || nameAtt == "" {
		return fmt.Errorf("manifest.json must contain a " +
			"non-empty 'name' attribute")
	}

	return nil
}

func checkManifestPort(manifest map[string]any) error {
	// Must have `ports`
	if _, ok := manifest["ports"]; !ok {
		return fmt.Errorf("manifest.json must contain `ports` attribute")
	}

	return nil
}

func checkManifestParam(manifest map[string]any) error {
	// Must have `parameters`
	if _, ok := manifest["parameters"]; !ok {
		return fmt.Errorf("manifest.json must contain `parameters` attribute")
	}

	return nil
}
