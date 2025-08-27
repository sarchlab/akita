package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/pkg/browser"
	"github.com/sarchlab/akita/v4/daisen/static"
)

var (
	httpFlag = flag.String("http",
		"localhost:3001",
		"HTTP service address (e.g., ':6060')")
	sqliteFileName = flag.String("sqlite",
		"",
		"Name of the SQLite file to read from.")

	traceReader *SQLiteTraceReader
	fs          http.FileSystem
)

func main() {
	parseArgs()

	fs = static.GetAssets()

	startServer()
}

func parseArgs() {
	flag.Parse()
}

func startServer() {
	connectToDB()
	startAPIServer()
}

func connectToDB() {
	if *sqliteFileName == "" {
		panic("Must specify a SQLite file")
	}

	traceReader = NewSQLiteTraceReader(*sqliteFileName)
	traceReader.Init()
}

func startAPIServer() {
	http.Handle("/", http.FileServer(fs))
	http.HandleFunc("/dashboard", serveIndex)
	http.HandleFunc("/component", serveIndex)
	http.HandleFunc("/task", serveIndex)

	http.HandleFunc("/api/trace", httpTrace)
	http.HandleFunc("/api/compnames", httpComponentNames)
	http.HandleFunc("/api/compinfo", httpComponentInfo)

	http.HandleFunc("/api/gpt", httpGPTProxy)
	http.HandleFunc("/api/githubisavailable", httpGithubIsAvailableProxy)
	http.HandleFunc("/api/checkenv", httpCheckEnvFile)
	// http.HandleFunc("/api/github", httpGithubProxy)

	fmt.Printf("Listening %s\n", *httpFlag)

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

	err := http.ListenAndServe(*httpFlag, nil)
	dieOnErr(err)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	var err error

	f, err := fs.Open("index.html")
	dieOnErr(err)

	p, err := io.ReadAll(f)
	dieOnErr(err)

	_, err = w.Write(p)
	dieOnErr(err)
}

func dieOnErr(err error) {
	if err != nil {
		log.Panic(err)
	}
}
