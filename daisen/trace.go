package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/sarchlab/akita/v3/tracing"
)

func httpTrace(w http.ResponseWriter, r *http.Request) {
	useTimeRange := true
	if r.FormValue("starttime") == "" || r.FormValue("endtime") == "" {
		useTimeRange = false
	}

	var err error
	startTime := 0.0
	endTime := 0.0
	if useTimeRange {
		startTime, err = strconv.ParseFloat(r.FormValue("starttime"), 64)
		if err != nil {
			panic(err)
		}

		endTime, err = strconv.ParseFloat(r.FormValue("endtime"), 64)
		if err != nil {
			panic(err)
		}
	}

	query := tracing.TaskQuery{
		ID:               r.FormValue("id"),
		ParentID:         r.FormValue("parentid"),
		Kind:             r.FormValue("kind"),
		Where:            r.FormValue("where"),
		StartTime:        startTime,
		EndTime:          endTime,
		EnableTimeRange:  useTimeRange,
		EnableParentTask: false,
	}

	tasks := traceReader.ListTasks(query)

	rsp, err := json.Marshal(tasks)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}


func httpDelayEvents(w http.ResponseWriter, r *http.Request) {
    source := r.FormValue("source")
    query := tracing.DelayQuery{
        Source: source,
    }

    delayEvents := traceReader.ListDelayEvents(query)
    delayEventsJSON, err := json.Marshal(delayEvents)
    if err != nil {
        http.Error(w, "Failed to marshal delay events to JSON", http.StatusInternalServerError)
        return
    }

    // Write the JSON response
    w.Header().Set("Content-Type", "application/json")
    _, err = w.Write(delayEventsJSON)
    if err != nil {
        http.Error(w, "Failed to write response", http.StatusInternalServerError)
        return
    }
}


func httpProgressEvents(w http.ResponseWriter, r *http.Request) {

    source := r.FormValue("source")
    query := tracing.ProgressQuery{
        Source: source,
    }
    progressEvents := traceReader.ListProgressEvents(query);
    rsp, err := json.Marshal(progressEvents)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    _, err = w.Write(rsp)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}

func httpDependencyEvents(w http.ResponseWriter, r *http.Request) {
    dependencyEvents := traceReader.ListDependencyEvents()
    rsp, err := json.Marshal(dependencyEvents)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    _, err = w.Write(rsp)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
}
