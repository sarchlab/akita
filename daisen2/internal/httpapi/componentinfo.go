package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
)

// TimeValue represents a data point with a time and a value.
type TimeValue struct {
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

// ComponentInfo contains time-series info for a component.
type ComponentInfo struct {
	Name      string      `json:"name"`
	InfoType  string      `json:"info_type"`
	StartTime float64     `json:"start_time"`
	EndTime   float64     `json:"end_time"`
	Data      []TimeValue `json:"data"`
}

// StackedTimeValue represents a data point with per-kind values.
type StackedTimeValue struct {
	Time   float64            `json:"time"`
	Values map[string]float64 `json:"values"` // key: milestone kind, value: count
}

// StackedComponentInfo contains stacked time-series info for a component.
type StackedComponentInfo struct {
	Name      string             `json:"name"`
	InfoType  string             `json:"info_type"`
	StartTime float64            `json:"start_time"`
	EndTime   float64            `json:"end_time"`
	Data      []StackedTimeValue `json:"data"`
	Kinds     []string           `json:"kinds"` // list of all milestone kinds
}

func (s *Server) httpComponentNames(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	writeJSON(w, s.traceReader.ListComponents(r.Context()))
}

func (s *Server) httpComponentInfo(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	compName := r.FormValue("where")
	infoType := r.FormValue("info_type")

	startTime, err := strconv.ParseFloat(r.FormValue("start_time"), 64)
	if err != nil {
		http.Error(w, "invalid start_time", http.StatusBadRequest)
		return
	}

	endTime, err := strconv.ParseFloat(r.FormValue("end_time"), 64)
	if err != nil {
		http.Error(w, "invalid end_time", http.StatusBadRequest)
		return
	}

	numDots, err := strconv.ParseInt(r.FormValue("num_dots"), 10, 32)
	if err != nil {
		http.Error(w, "invalid num_dots", http.StatusBadRequest)
		return
	}

	var compInfo *ComponentInfo

	switch infoType {
	case "ReqInCount":
		compInfo = s.calculateReqIn(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "ReqCompleteCount":
		compInfo = s.calculateReqComplete(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "AvgLatency":
		compInfo = s.calculateAvgLatency(
			r.Context(), compName, startTime, endTime, int(numDots))
	case "ConcurrentTask":
		compInfo = s.calculateConcurrentTask(
			r.Context(), compName, infoType, startTime, endTime, numDots)
	case "ConcurrentTaskMilestones":
		s.httpConcurrentTaskMilestones(w, r, compName, infoType, startTime, endTime, int(numDots))
		return
	case "RequestBufferPressure":
		compInfo = s.calculateRequestBufferPressure(
			r.Context(), compName, infoType, startTime, endTime, numDots)
	case "ResponseBufferPressure":
		compInfo = s.calculateResponseBufferPressure(
			r.Context(), compName, infoType, startTime, endTime, numDots)
	case "PendingReqOut":
		compInfo = s.calculatePendingReqOut(
			r.Context(), compName, infoType, startTime, endTime, numDots)
	default:
		http.Error(w, "unknown info_type", http.StatusBadRequest)
		return
	}

	writeJSON(w, compInfo)
}

// httpConcurrentTaskMilestones writes the blocking-reason chart — the one metric
// the scoped detail view uses, so it honors a location subtree. The scope falls
// back to the component name, and a leaf scope matches only itself.
func (s *Server) httpConcurrentTaskMilestones(
	w http.ResponseWriter,
	r *http.Request,
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
) {
	scope := r.FormValue("scope")
	if scope == "" {
		scope = compName
	}
	// sample is a 1-in-N task stride for a fast, approximate preview (default 1 =
	// exact), so the client can show a coarse blocking-reason chart immediately.
	sample := 1
	if v := r.FormValue("sample"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 1 {
			sample = n
		}
	}
	// group=kind colors blocking reasons by milestone kind alone; anything else
	// (default) keeps the finer kind-what grouping, mirroring component_timeline so
	// the legend matches the client's colorMode.
	groupByKind := r.FormValue("group") == "kind"
	stackedInfo := s.calculateConcurrentTaskMilestones(
		r.Context(), scope, infoType, startTime, endTime, numDots, sample, groupByKind)
	writeJSON(w, stackedInfo)
}

func (s *Server) calculateConcurrentTask(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo := &ComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return compInfo
	}

	// Occupancy needs only the task intervals, so fetch just those (one covering
	// index scan) instead of hydrating every Task via ListTasks, then run the same
	// time-weighted bin sweep. Drops this metric from ~2s to ~0.1s on a large
	// component by avoiding the per-task struct/parent hydration.
	tasks := s.traceReader.listTaskIntervals(ctx, compName, "", nil, startTime, endTime)
	binDuration := totalDuration / float64(numDots)
	compInfo.Data = calculateTimeWeightedBins(
		tasks, startTime, endTime, binDuration, int(numDots),
		func(t Task) bool { return true },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func (s *Server) calculateRequestBufferPressure(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	// An incoming port buffer holds both the requests a component receives and the
	// responses to requests it sent out (e.g. a pure client like the mem agent only
	// ever buffers responses). akita names message types "*Req"/"*Request" and
	// "*Rsp"/"*Response", so split the incoming_buffer occupancy by that: the
	// requests waiting to be served.
	return s.calculateKindOccupancy(
		ctx, compName, infoType, "incoming_buffer", []string{"%Req", "%Request"}, startTime, endTime, numDots)
}

func (s *Server) calculateResponseBufferPressure(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	// The other half of the incoming buffer: responses returning for requests this
	// component sent downstream.
	return s.calculateKindOccupancy(
		ctx, compName, infoType, "incoming_buffer", []string{"%Rsp", "%Response"}, startTime, endTime, numDots)
}

func (s *Server) calculatePendingReqOut(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	// A "req_out" task spans an outgoing request from issue to its response, so the
	// number in flight is how many requests the component still has pending
	// downstream — the pending-request-out level. (The outgoing-buffer tasks are a
	// poor proxy here: a request usually leaves the port the instant it is accepted,
	// so those tasks are near-zero duration.)
	return s.calculateKindOccupancy(
		ctx, compName, infoType, "req_out", nil, startTime, endTime, numDots)
}

// calculateKindOccupancy bins the time-weighted count of in-flight tasks of one
// kind (optionally narrowed to one of the What LIKE patterns) over the scope's
// subtree. Like calculateConcurrentTask it fetches only the intervals — one
// index-driven scan, no per-task hydration — and runs the shared bin sweep, so it
// is cheap even on a busy component.
func (s *Server) calculateKindOccupancy(
	ctx context.Context,
	compName, infoType, kind string,
	whatLikes []string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo := &ComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return compInfo
	}

	// The (Location, Kind, StartTime, EndTime) covering index answers the scoped,
	// kind-filtered interval scan without touching the trace table.
	s.traceReader.ensureIndex(
		ctx, "Building index idx_trace_loc_kind_times", metricCoveringIndex)

	tasks := s.traceReader.listTaskIntervals(ctx, compName, kind, whatLikes, startTime, endTime)
	binDuration := totalDuration / float64(numDots)
	compInfo.Data = calculateTimeWeightedBins(
		tasks, startTime, endTime, binDuration, int(numDots),
		func(t Task) bool { return true },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func (s *Server) calculateReqIn(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "req_in",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedEventRate(ctx, info.Data,
		compName, "req_in", "StartTime",
		startTime, endTime, binDuration)

	return info
}

func (s *Server) calculateReqComplete(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "req_complete",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedEventRate(ctx, info.Data,
		compName, "req_in", "EndTime",
		startTime, endTime, binDuration)

	return info
}

func (s *Server) calculateAvgLatency(
	ctx context.Context,
	compName string,
	startTime, endTime float64,
	numDots int,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  "avg_latency",
		StartTime: startTime,
		EndTime:   endTime,
	}

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = buildEmptyTimeValues(startTime, binDuration, numDots)

	if binDuration <= 0 {
		return info
	}

	s.fillBinnedAverageLatency(ctx, info.Data,
		compName, startTime, endTime, binDuration)

	return info
}

func buildEmptyTimeValues(startTime, binDuration float64, numDots int) []TimeValue {
	data := make([]TimeValue, numDots)
	for i := range data {
		binStartTime := float64(i)*binDuration + startTime
		data[i] = TimeValue{
			Time: binStartTime + 0.5*binDuration,
		}
	}
	return data
}

// metricCoveringIndex covers the columns the scoped, kind-filtered metric queries
// read, so a multi-facet component aggregate seeks straight to (Location, Kind)
// and scans only the matching facet's rows from the index — turning a ~0.5s
// subtree scan into ~0.02s. Built once (IF NOT EXISTS) on first metric query.
const metricCoveringIndex = `
CREATE INDEX IF NOT EXISTS idx_trace_loc_kind_times ON trace(Location, Kind, StartTime, EndTime)`

func (s *Server) fillBinnedEventRate(
	ctx context.Context,
	data []TimeValue,
	compName, kind, timeColumn string,
	startTime, endTime, binDuration float64,
) {
	timeColumn = safeTraceTimeColumn(timeColumn)
	// Best-effort: ensure the covering index (a no-op once it exists; ignored on a
	// read-only connection — the query then just runs slower against the table).
	s.traceReader.ensureIndex(
		ctx, "Building index idx_trace_loc_kind_times", metricCoveringIndex)
	// Scope semantics: match the named location OR anything nested under it, so a
	// component name (e.g. "AT") aggregates its whole subtree while a leaf row
	// matches only itself.
	lo, hi := scopePrefixBounds(compName)
	query := fmt.Sprintf(`
		SELECT CAST(((%s - ?) / ?) AS INTEGER) AS Bin, COUNT(*)
		FROM trace
		WHERE Location IN (SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?))
			AND Kind = ? AND %s > ? AND %s < ?
		GROUP BY Bin
	`, timeColumn, timeColumn, timeColumn)

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, lo, hi, kind, startTime, endTime,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var bin int
		var count int64
		if err := rows.Scan(&bin, &count); err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}

		if bin >= 0 && bin < len(data) {
			data[bin].Value = float64(count) / binDuration
		}
	}
	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
}

func (s *Server) fillBinnedAverageLatency(
	ctx context.Context,
	data []TimeValue,
	compName string,
	startTime, endTime, binDuration float64,
) {
	s.traceReader.ensureIndex(
		ctx, "Building index idx_trace_loc_kind_times", metricCoveringIndex)
	// Scope semantics: aggregate the named location's whole subtree (a leaf still
	// matches only itself).
	lo, hi := scopePrefixBounds(compName)
	query := `
		SELECT CAST(((EndTime - ?) / ?) AS INTEGER) AS Bin,
			AVG(EndTime - StartTime)
		FROM trace
		WHERE Location IN (SELECT ID FROM location WHERE Locale = ? OR (Locale >= ? AND Locale < ?))
			AND Kind = 'req_in' AND EndTime > ? AND EndTime < ?
		GROUP BY Bin
	`

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, lo, hi, startTime, endTime,
	)
	if err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
	defer rows.Close()

	for rows.Next() {
		var bin int
		var value float64
		if err := rows.Scan(&bin, &value); err != nil {
			if ctx.Err() != nil {
				return
			}
			panic(err)
		}

		if bin >= 0 && bin < len(data) {
			data[bin].Value = value
		}
	}
	if err := rows.Err(); err != nil {
		if ctx.Err() != nil {
			return
		}
		panic(err)
	}
}

func safeTraceTimeColumn(column string) string {
	switch column {
	case "StartTime", "EndTime":
		return column
	default:
		panic(fmt.Sprintf("unsupported trace time column %q", column))
	}
}

type taskFilter func(t Task) bool
type taskTime func(t Task) float64

type countEvent struct {
	time  float64
	delta int
}

func calculateTimeWeightedBins( //nolint:funlen
	tasks []Task,
	startTime, endTime, binDuration float64,
	numDots int,
	filter taskFilter,
	increaseTime, decreaseTime taskTime,
) []TimeValue {
	data := buildEmptyTimeValues(startTime, binDuration, numDots)
	events := make([]countEvent, 0, len(tasks)*2)
	count := 0

	for _, task := range tasks {
		if !filter(task) {
			continue
		}

		start := increaseTime(task)
		end := decreaseTime(task)
		if end <= startTime || start >= endTime || end <= start {
			continue
		}

		if start < startTime {
			count++
		} else {
			events = append(events, countEvent{time: start, delta: 1})
		}

		if end <= endTime {
			events = append(events, countEvent{time: end, delta: -1})
		}
	}

	sort.Slice(events, func(i, j int) bool {
		return events[i].time < events[j].time
	})

	eventIndex := 0
	for i := range data {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		for eventIndex < len(events) && events[eventIndex].time <= binStartTime {
			count += events[eventIndex].delta
			eventIndex++
		}

		timeByCount := 0.0
		prevTime := binStartTime
		for eventIndex < len(events) && events[eventIndex].time < binEndTime {
			eventTime := events[eventIndex].time
			if eventTime > prevTime {
				timeByCount += (eventTime - prevTime) * float64(count)
				prevTime = eventTime
			}

			for eventIndex < len(events) && events[eventIndex].time == eventTime {
				count += events[eventIndex].delta
				eventIndex++
			}
		}

		timeByCount += (binEndTime - prevTime) * float64(count)
		data[i].Value = timeByCount / binDuration
	}

	return data
}

func (s *Server) calculateConcurrentTaskMilestones(
	ctx context.Context,
	scope, infoType string,
	startTime, endTime float64,
	numDots, sample int,
	groupByKind bool,
) *StackedComponentInfo {
	info := &StackedComponentInfo{
		Name:      scope,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
		Kinds:     []string{},
		Data:      []StackedTimeValue{},
	}

	if numDots < 1 || endTime <= startTime {
		return info
	}

	// Same occupancy binning as the task-count chart, grouped by blocking reason
	// (milestone kind, or the finer kind-what) instead of task kind — computed in
	// SQL, no per-task fetch.
	keys, bins := s.traceReader.BlockingReasonOccupancy(
		ctx, scope, startTime, endTime, numDots, sample, groupByKind)
	info.Kinds = keys

	binWidth := (endTime - startTime) / float64(numDots)
	data := make([]StackedTimeValue, 0, len(bins))
	for b, row := range bins {
		values := make(map[string]float64, len(keys))
		for ki, k := range keys {
			values[k] = float64(row[ki])
		}
		data = append(data, StackedTimeValue{
			// Keep the bin-center sample time the bar chart already plots against.
			Time:   startTime + (float64(b)+0.5)*binWidth,
			Values: values,
		})
	}
	info.Data = data

	return info
}
