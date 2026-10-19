package main

import (
	"fmt"
	"os"

	"github.com/sarchlab/akita/v5/akita/internal/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}
