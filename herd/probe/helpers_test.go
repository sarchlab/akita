package probe

import "github.com/sarchlab/akita/v5/herd/internal/testutil"

// Short names of the shared test helpers.
var (
	writeFile  = testutil.WriteFile
	writeFiles = testutil.WriteFiles
	readFile   = testutil.ReadFile
)
