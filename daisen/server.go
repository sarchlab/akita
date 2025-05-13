package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sarchlab/akita/v4/daisen/static"
	"github.com/sarchlab/akita/v4/datarecording"
)

var (
	httpFlag = flag.String("http",
		"0.0.0.0:3001",
		"HTTP service address (e.g., ':6060')")
	sqliteFileName = flag.String("sqlite",
		"",
		"Name of the SQLite file to read from.")

	db datarecording.DataReader
	fs http.FileSystem
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

	db = datarecording.NewReader(*sqliteFileName)
}

func startAPIServer() {
	http.Handle("/", http.FileServer(fs))
	http.HandleFunc("/dashboard", serveIndex)
	http.HandleFunc("/component", serveIndex)
	http.HandleFunc("/task", serveIndex)

	http.HandleFunc("/api/trace", httpTrace)
	http.HandleFunc("/api/compnames", httpComponentNames)
	http.HandleFunc("/api/compinfo", httpComponentInfo)

	fmt.Printf("Listening %s\n", *httpFlag)

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
