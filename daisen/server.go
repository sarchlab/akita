package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"

	_ "github.com/go-sql-driver/mysql"
	"github.com/sarchlab/akita/v3/daisen/static"
	"github.com/sarchlab/akita/v3/tracing"
)

var (
	httpFlag = flag.String("http",
		"0.0.0.0:3001",
		"HTTP service address (e.g., ':6060')")
	mySQLDBName = flag.String("mysql",
		"",
		"Name of the MySQL database to connect to.")
	csvFileName = flag.String("csv",
		"",
		"Name of the CSV file to read from.")
	sqliteFileName = flag.String("sqlite",
		"",
		"Name of the SQLite file to read from.")

	traceReader tracing.TraceReader
	fs          http.FileSystem
)

func main() {
	parseArgs()
	fs = static.GetAssets()
	startServer()
}

func parseArgs() {
	flag.Parse()

	mustBeOneAndOnlyOneSource()
}

func mustBeOneAndOnlyOneSource() {
	numSources := 0
	if *mySQLDBName != "" {
		numSources++
	}

	if *csvFileName != "" {
		numSources++
	}

	if *sqliteFileName != "" {
		numSources++
	}

	if numSources != 1 {
		flag.PrintDefaults()
		panic("Must specify one and only one of -mysql, -csv, or -sqlite")
	}
}

func startServer() {
	connectToDB()
	startAPIServer()
}

func connectToDB() {
	switch {
	case *mySQLDBName != "":
		db := tracing.NewMySQLTraceReader(*mySQLDBName)
		db.Init()
		traceReader = db
	case *csvFileName != "":
		db := tracing.NewCSVTraceReader(*csvFileName)
		traceReader = db
	case *sqliteFileName != "":
		db := tracing.NewSQLiteTraceReader(*sqliteFileName)
		db.Init()
		traceReader = db
	}
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
