package cmd

import (
	_ "embed"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var linterCmd = &cobra.Command{
	Use:   "check",
	Short: "Check component format.",
	Long:  "Use `check [component folder path]` to check the format of the component in the filder.",
	Run: func(cmd *cobra.Command, args []string) {
		folderPath := args[0]

		err := checkComponentFormat(folderPath)
		if err != nil {
			log.Fatalf("Validation failed: %s", err)
		} else {
			log.Fatalf("Validation succeed!")
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
		return fmt.Errorf("comp file does not exist")
	}

	// parse the comp file and
	fs := token.NewFileSet()
	node, err := parser.ParseFile(fs, compFilePath, nil, 0)
	if err != nil {
		return fmt.Errorf("failed to parse comp file %s: %v", compFilePath, err)
	}

	existCompStruct := false
	for _, decl := range node.Decls { // iterate all declaration
		genDecl, ok := decl.(*ast.GenDecl)    // check if declaration one of the GenDecl
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
		return fmt.Errorf("no type Comp struct in comp.go")
	}

	return nil
}
