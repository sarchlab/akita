package ping_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTutorialSnippetsMatchRunnableExamples(t *testing.T) {
	for _, pair := range [][2]string{
		{"example_test.go", "01_manual_hardware.md"},
		{"blueprint_example_test.go", "02_blueprint_hardware.md"},
		{"workload/workload.go", "01_manual_hardware.md"},
	} {
		source, err := os.ReadFile(pair[0])
		require.NoError(t, err)
		tutorial, err := os.ReadFile("../../doc/tutorial/configuration/" + pair[1])
		require.NoError(t, err)
		require.Contains(t, string(tutorial), "```go\n"+string(source)+"```")
	}
}
