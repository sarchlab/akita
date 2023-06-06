package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"

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
	http.HandleFunc("/", serveStatic)
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

func serveStatic(w http.ResponseWriter, r *http.Request) {
	devServerURL, devMode := devServer()

	if devMode {
		url := devServerURL + r.URL.Path
		rsp, err := http.Get(url)
		dieOnErr(err)
		defer func() {
			innerErr := rsp.Body.Close()
			dieOnErr(innerErr)
		}()

		if rsp.StatusCode != http.StatusOK {
			http.NotFound(w, r)
			return
		}

		body, err := ioutil.ReadAll(rsp.Body)
		dieOnErr(err)

		_, err = w.Write(body)
		dieOnErr(err)

		return
	}

	f, err := fs.Open(r.URL.Path)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	p, err := io.ReadAll(f)
	dieOnErr(err)

	_, err = w.Write(p)
	dieOnErr(err)
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	devServerURL, devMode := devServer()

	if devMode {
		rsp, err := http.Get(devServerURL + "/index.html")
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer func() {
			innerErr := rsp.Body.Close()
			dieOnErr(innerErr)
		}()

		_, err = io.Copy(w, rsp.Body)
		dieOnErr(err)

		return
	}

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

func devServer() (string, bool) {
	evName := "AKITA_DAISEN_DEV_SERVER"
	evValue, exist := os.LookupEnv(evName)

	if !exist {
		return "", false
	}

	return evValue, true
}
