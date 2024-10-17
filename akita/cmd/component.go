/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
	"github.com/sarchlab/akita/v4/sim"
)

var create bool

// createCompCmd represents the createComp command
var componentCmd = &cobra.Command{
	Use:   "component",
	Short: "Create a new component",
	Long: `Akita allows developers to create new component
	and create and integrate new simulator components.
	This command works to generate the needed files
    to quickly create a new component.`,
    Args:  cobra.ExactArgs(1),

	Run: func(cmd *cobra.Command, args []string) {
	    if create {
        	componentName := args[0]
        	newComponent := sim.NewComponentBase(componentName)
        	fmt.Printf("Component '%s' created successfully.\n", newComponent.Name())
        } else {
        	fmt.Println("Use --create <component_name> to create a component.")
        }
	},
}

func init() {
	componentCmd.Flags().BoolVarP(&create, "create", "", false, "Create a new component")
    //componentCmd.Flags().StringVarP(&componentName, "create", "", "", "Name of the component to create")

	rootCmd.AddCommand(componentCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// createCompCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	//componentCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
