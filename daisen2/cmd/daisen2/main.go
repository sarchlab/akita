// Command daisen2 starts the Daisen2 trace viewer in replay mode.
package main

import (
	"flag"
	"fmt"
	"log"
	"os/exec"
	"runtime"

	"github.com/pkg/browser"
	"github.com/sarchlab/akita/v5/daisen2"
)

var (
	httpFlag = flag.String("http",
		"localhost:3001",
		"HTTP service address (e.g., ':6060')")
	sqliteFileName = flag.String("sqlite",
		"",
		"Name of the SQLite file to read from.")
	openFlag = flag.Bool("open",
		false,
		"Open the dashboard in a browser tab on start (off by default).")
)

func main() {
	flag.Parse()

	if *sqliteFileName == "" {
		log.Fatal("Must specify a SQLite file with -sqlite flag")
	}

	server := daisen2.NewReplayServer(*sqliteFileName, *httpFlag)

	url := fmt.Sprintf("http://localhost%s", *httpFlag)
	if len(*httpFlag) > 0 && (*httpFlag)[0] != ':' {
		url = fmt.Sprintf("http://%s", *httpFlag)
	}

	if *openFlag {
		go func() {
			if err := openBrowserInBackground(url); err != nil {
				log.Printf("Error opening browser: %v\n", err)
			}
		}()
	} else {
		log.Printf("Serving at %s — pass --open to open it in a browser tab", url)
	}

	server.Start()
}

// openBrowserInBackground opens url in the user's default browser without raising
// the browser over the current app. On macOS `open -g` opens in the background so
// keyboard focus stays where it is; other platforms fall back to the default
// opener (which may bring the browser to the foreground).
func openBrowserInBackground(url string) error {
	if runtime.GOOS == "darwin" {
		return exec.Command("open", "-g", url).Start()
	}
	return browser.OpenURL(url)
}
