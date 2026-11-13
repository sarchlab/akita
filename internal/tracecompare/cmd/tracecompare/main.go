// Command tracecompare compares two Akita vis-trace databases and exits with
// status 1 if they differ.
//
//	tracecompare [-ignore-ids] first.sqlite3 second.sqlite3
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/sarchlab/akita/v5/internal/tracecompare"
)

func main() {
	ignoreIDs := flag.Bool("ignore-ids", false,
		"Compare tasks by content and parent content instead of by ID")
	flag.Parse()

	if flag.NArg() != 2 {
		fmt.Fprintln(os.Stderr,
			"usage: tracecompare [-ignore-ids] first.sqlite3 second.sqlite3")
		os.Exit(2)
	}

	mode := tracecompare.Exact
	if *ignoreIDs {
		mode = tracecompare.IgnoreIDs
	}

	result, err := tracecompare.Compare(flag.Arg(0), flag.Arg(1), mode)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	fmt.Printf("tasks: %d vs %d, differing entries: %d\n",
		result.NumTasks[0], result.NumTasks[1], result.NumDiffs)

	for _, d := range result.Diffs {
		fmt.Println("  " + d)
	}

	if !result.Equal() {
		os.Exit(1)
	}
}
