package workspace

import "github.com/sarchlab/akita/v5/experiment/internal/testutil"

// Short names of the shared test helpers.
var (
	writeFiles = testutil.WriteFiles
	readFile   = testutil.ReadFile
	exists     = testutil.Exists
)
