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
	Long:  "Use `check [component folder path]` to check the format of the component in the filder.",
	Run: func(cmd *cobra.Command, args []string) {
		folderPath := args[0]

		hasError := false
		errCompStruct := checkComponentFormat(folderPath)
		if errCompStruct != nil {
			fmt.Printf("<1a> Component structure error: %s\n", errCompStruct)
			hasError = true
		}

		errBuilderStruct := checkBuilderStruct(folderPath)
		if errBuilderStruct != nil {
			fmt.Printf("<2a> Builder structure error: %s\n", errBuilderStruct)
			hasError = true
		}

		errBuilder := checkBuilderFormat(folderPath)
		if errBuilder != nil {
			fmt.Printf("<2b> Builder format error: %s\n", errBuilder)
			hasError = true
		}

		errBuilderReturn := checkBuilderReturn(folderPath)
		if errBuilderReturn != nil {
			fmt.Printf("<2b> Builder return error: %s\n", errBuilderReturn)
			hasError = true
		}

		errBuilderParameter := checkBuilderParameters(folderPath)
		if errBuilderParameter != nil {
			fmt.Printf("<2c> Builder parameter error: %s\n", errBuilderParameter)
			hasError = true
		}

		errBuilderFunc := checkBuilderFunction(folderPath)
		if errBuilderFunc != nil {
			fmt.Printf("<2d> Builder function error: %s\n", errBuilderFunc)
			hasError = true
		}

		errManifest := checkManifestFormat(folderPath)
		if errManifest != nil {
			fmt.Printf("<3a/b> Manifest error: %v", errManifest)
			hasError = true
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
		return fmt.Errorf("comp.go file does not exist")
	}

	// parse the comp file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, compFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse comp.go file %s: %v", compFilePath, err)
	}

	existCompStruct := false
	for _, decl := range node.Decls { // iterate all declaration
		genDecl, ok := decl.(*ast.GenDecl)    // check if declaration is one of the GenDecl
		if !ok || genDecl.Tok != token.TYPE { // check if declaration is a type decl
			continue
		}
		for _, spec := range genDecl.Specs { // iterate specs in the type decl
			typeSpec, ok := spec.(*ast.TypeSpec)    //check if spec is one of the Expr
			if ok && typeSpec.Name.Name == "Comp" { // check struct name
				if _, isStruct := typeSpec.Type.(*ast.StructType); isStruct {
					existCompStruct = true
					break
				}
			}
		}
	}
	if !existCompStruct {
		return fmt.Errorf("no Comp struct in comp.go")
	}

	return nil
}

func checkBuilderStruct(folderPath string) error {
	// check builder.go existence
	builderFilePath := filepath.Join(folderPath, "builder.go")
	if _, err := os.Stat(builderFilePath); os.IsNotExist(err) {
		return fmt.Errorf("builder.go file does not exist")
	}

	// parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse builder.go file %s: %v", builderFilePath, err)
	}

	existBuilderStruct := false
	for _, decl := range node.Decls { // iterate all declaration
		genDecl, ok := decl.(*ast.GenDecl)    // check if declaration is one of the GenDecl
		if !ok || genDecl.Tok != token.TYPE { // check if declaration is a type decl
			continue
		}
		for _, spec := range genDecl.Specs { // iterate specs in the type decl
			typeSpec, ok := spec.(*ast.TypeSpec)       //check if spec is one of the Expr
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

// Logic: if the parameter in the builder has a setter statement in a `With` function or has no setter at all, pass;
// if it has a setter statement but located in a func not named by `With...`, return error.
func checkBuilderFormat(folderPath string) error {
	// check builder.go existence
	builderFilePath := filepath.Join(folderPath, "builder.go")
	if _, err := os.Stat(builderFilePath); os.IsNotExist(err) {
		return fmt.Errorf("builder.go file does not exist")
	}

	// parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse builder.go file %s: %v", builderFilePath, err)
	}

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
					builderFields[fieldName.Name] = true // Assume all parameters are configurable
					configurableFields[fieldName.Name] = true
				}
			}
		}
	}

	// find the object of all configuration func
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}

		// record receiver name
		receiverName := ""
		if funcDecl.Recv != nil && len(funcDecl.Recv.List) == 1 {
			if len(funcDecl.Recv.List[0].Names) == 1 {
				receiverName = funcDecl.Recv.List[0].Names[0].Name
			}
		}

		// find assignment receiver.<field> = ...
		ast.Inspect(funcDecl.Body, func(n ast.Node) bool { // iterate every node(statement)
			assign, ok := n.(*ast.AssignStmt) // check if statement is an assignment
			if !ok {
				return true // continue to iterate every subnode of the node
			}
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok { // if left is a selector expression
					ident, ok := sel.X.(*ast.Ident)
					if ok && ident.Name == receiverName {
						fieldName := sel.Sel.Name
						//fmt.Println(funcDecl.Name.Name)
						if builderFields[fieldName] && !strings.HasPrefix(funcDecl.Name.Name, "With") {
							configurableFields[fieldName] = false
						}
					}
				}
			}
			return true
		})

		var unconfigs []string
		for key, value := range configurableFields {
			if !value {
				unconfigs = append(unconfigs, key)
			}
		}

		if len(unconfigs) != 0 {
			unconfig := strings.Join(unconfigs, ", ")
			return fmt.Errorf("configurable parameter(s) [%s] does not have proper setter functions starting with 'With'", unconfig)
		}
	}

	return nil
}

func checkBuilderReturn(folderPath string) error {
	builderFilePath := filepath.Join(folderPath, "builder.go")

	// parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse builder.go file %s: %v", builderFilePath, err)
	}

	var improperReturns []string
	for _, decl := range node.Decls { // iterate all declaration
		funcDecl, ok := decl.(*ast.FuncDecl) // check if declaration is a FuncDecl
		if !ok || funcDecl.Recv == nil || funcDecl.Name == nil {
			continue
		}
		if !strings.HasPrefix(funcDecl.Name.Name, "With") { //if func name start with With
			continue
		}
		if funcDecl.Type.Results == nil {
			continue
		}
		for _, result := range funcDecl.Type.Results.List {
			if ident, ok := result.Type.(*ast.Ident); !ok || ident.Name != "Builder" {
				improperReturns = append(improperReturns, funcDecl.Name.Name)
			}
		}
	}
	if len(improperReturns) != 0 {
		funcList := strings.Join(improperReturns, ", ")
		return fmt.Errorf("'With' function(s) [%s] does not return builder type value", funcList)
	}

	return nil
}

func checkBuilderParameters(folderPath string) error {
	builderFilePath := filepath.Join(folderPath, "builder.go")

	// Parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse builder.go file %s: %v", builderFilePath, err)
	}

	var parameters []string
	var mustInclude = 0

	// Find all field in Builder struct
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
					if fieldName.Name == "Freq" || fieldName.Name == "Engine" {
						mustInclude += 1
					}
					parameters = append(parameters, fieldName.Name)
				}
			}
		}
	}

	if len(parameters) < 2 || mustInclude != 2 {
		return fmt.Errorf("builder must include at least 2 parameters, including `Freq` and `Engine`")
	}

	return nil
}

func checkBuilderFunction(folderPath string) error {
	builderFilePath := filepath.Join(folderPath, "builder.go")

	// Parse the builder file
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, builderFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse builder.go file %s: %v", builderFilePath, err)
	}

	found := false
	for _, decl := range node.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !ok || funcDecl.Name.Name != "Build" {
			continue
		}
		found = true

		// Check if func has exactly one parameter
		if funcDecl.Type.Params.NumFields() != 1 {
			return fmt.Errorf("`Build` function must take exactly one argument")
		}

		// Check if the parameter type is string
		param := funcDecl.Type.Params.List[0]
		ident, ok := param.Type.(*ast.Ident)
		if !ok || ident.Name != "string" {
			return fmt.Errorf("`Build` function's argument must be of type string")
		}

		// Check if func returns
		if funcDecl.Type.Results == nil {
			return fmt.Errorf("`Build` function must return the new component")
		}

		// Check if the return type is a pointer
		retType := funcDecl.Type.Results.List[0].Type
		_, ok = retType.(*ast.StarExpr)
		if !ok {
			return fmt.Errorf("`Build` function must return the new component as a pointer type")
		}
	}

	if !found {
		return fmt.Errorf("`Build` function not found in builder")
	}

	return nil
}

func checkManifestFormat(folderPath string) error {
	// Check manifest.json existence
	jsonFilePath := filepath.Join(folderPath, "manifest.json")
	if _, err := os.Stat(jsonFilePath); os.IsNotExist(err) {
		return fmt.Errorf("manifest.json file does not exist")
	}

	// Read the json file
	fileContent, err := os.ReadFile(jsonFilePath)
	if err != nil {
		return fmt.Errorf("failed to read manifest.json: %v", err)
	}

	// Parse the json file
	var manifest map[string]any
	if err := json.Unmarshal(fileContent, &manifest); err != nil {
		return fmt.Errorf("failed to parse manifest.json: %v", err)
	}

	// Must have `name` attribute with a non-empty string value
	nameAtt, ok := manifest["name"].(string)
	if !ok || nameAtt == "" {
		return fmt.Errorf("manifest.json must contain a non-empty 'name' attribute")
	}

	// Must have `ports`
	if _, ok := manifest["ports"]; !ok {
		return fmt.Errorf("manifest.json must contain `ports` attribute")
	}

	// Must have `parameters`
	if _, ok := manifest["parameters"]; !ok {
		return fmt.Errorf("manifest.json must contain `parameters` attribute")
	}

	// All checks passed
	return nil
}
