// Command daisen starts the Daisen trace viewer in replay mode.
package main

import (
	"flag"
	"fmt"
	"log"

	"github.com/pkg/browser"
	"github.com/sarchlab/akita/v5/daisen"
)

var (
	httpFlag = flag.String("http",
		"localhost:3001",
		"HTTP service address (e.g., ':6060')")
	sqliteFileName = flag.String("sqlite",
		"",
		"Name of the SQLite file to read from.")
)

func main() {
	flag.Parse()

	if *sqliteFileName == "" {
		log.Fatal("Must specify a SQLite file with -sqlite flag")
	}

	server := daisen.NewReplayServer(*sqliteFileName, *httpFlag)

	go func() {
		url := fmt.Sprintf("http://localhost%s", *httpFlag)
		if (*httpFlag)[0] != ':' {
			url = fmt.Sprintf("http://%s", *httpFlag)
		}

		err := browser.OpenURL(url)
		if err != nil {
			log.Printf("Error opening browser: %v\n", err)
		}
	}()

	server.Start()
}
