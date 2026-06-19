package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
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

	componentNames := s.traceReader.ListComponents(r.Context())

	rsp, err := json.Marshal(componentNames)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func (s *Server) httpComponentInfo(w http.ResponseWriter, r *http.Request) {
	if s.traceReader == nil {
		http.Error(w, "trace data not available", http.StatusServiceUnavailable)
		return
	}

	compName := r.FormValue("where")
	infoType := r.FormValue("info_type")

	startTime, err := strconv.ParseFloat(r.FormValue("start_time"), 64)
	dieOnErr(err)

	endTime, err := strconv.ParseFloat(r.FormValue("end_time"), 64)
	dieOnErr(err)

	numDots, err := strconv.ParseInt(r.FormValue("num_dots"), 10, 32)
	dieOnErr(err)

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
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	case "ConcurrentTaskMilestones":
		stackedInfo := s.calculateConcurrentTaskMilestones(
			r.Context(), compName, infoType, startTime, endTime, int(numDots))
		rsp, err := json.Marshal(stackedInfo)
		dieOnErr(err)
		_, err = w.Write(rsp)
		dieOnErr(err)
		return
	case "BufferPressure":
		compInfo = s.calculateBufferPressure(
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	case "PendingReqOut":
		compInfo = s.calculatePendingReqOut(
			r.Context(), compInfo, compName, infoType, startTime, endTime, numDots)
	default:
		log.Panicf("unknown info_type %s\n", infoType)
	}

	rsp, err := json.Marshal(compInfo)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func (s *Server) calculateConcurrentTask(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		false,
		func(t Task) bool { return true },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func (s *Server) calculateBufferPressure(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		true,
		taskIsReqIn,
		func(t Task) float64 {
			return float64(t.ParentTask.StartTime)
		},
		func(t Task) float64 {
			return float64(t.StartTime)
		},
	)

	return compInfo
}

func (s *Server) calculatePendingReqOut(
	ctx context.Context,
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = s.calculateTimeWeightedTaskCount(
		ctx, compName, infoType,
		startTime, endTime, int(numDots),
		false,
		func(t Task) bool { return t.Kind == "req_out" },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func taskIsReqIn(t Task) bool {
	return t.Kind == "req_in" && t.ParentTask != nil
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

func (s *Server) fillBinnedEventRate(
	ctx context.Context,
	data []TimeValue,
	compName, kind, timeColumn string,
	startTime, endTime, binDuration float64,
) {
	timeColumn = safeTraceTimeColumn(timeColumn)
	query := fmt.Sprintf(`
		SELECT CAST(((%s - ?) / ?) AS INTEGER) AS Bin, COUNT(*)
		FROM trace
		WHERE Location = (SELECT ID FROM location WHERE Locale = ?)
			AND Kind = ? AND %s > ? AND %s < ?
		GROUP BY Bin
	`, timeColumn, timeColumn, timeColumn)

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, kind, startTime, endTime,
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
}

func (s *Server) fillBinnedAverageLatency(
	ctx context.Context,
	data []TimeValue,
	compName string,
	startTime, endTime, binDuration float64,
) {
	query := `
		SELECT CAST(((EndTime - ?) / ?) AS INTEGER) AS Bin,
			AVG(EndTime - StartTime)
		FROM trace
		WHERE Location = (SELECT ID FROM location WHERE Locale = ?)
			AND Kind = 'req_in' AND EndTime > ? AND EndTime < ?
		GROUP BY Bin
	`

	rows, err := s.traceReader.QueryContext(
		ctx, query,
		startTime, binDuration,
		compName, startTime, endTime,
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

func (s *Server) calculateTimeWeightedTaskCount(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
	enableParentTask bool,
	filter taskFilter,
	increaseTime, decreaseTime taskTime,
) *ComponentInfo {
	info := &ComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
	}

	query := TaskQuery{
		Where:            compName,
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: enableParentTask,
	}
	tasks := s.traceReader.ListTasks(ctx, query)

	totalDuration := endTime - startTime
	if numDots <= 0 || totalDuration <= 0 {
		return info
	}

	binDuration := totalDuration / float64(numDots)
	info.Data = calculateTimeWeightedBins(
		tasks, startTime, endTime, binDuration, numDots,
		filter, increaseTime, decreaseTime)

	return info
}

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

// maxTraceContextRows caps how many trace events get embedded in the chat
// context. The full trace can be hundreds of thousands of rows, which both pegs
// the server (building the string) and overruns the model's context window, so
// we send only a bounded sample.
const maxTraceContextRows = 500

func buildTraceSQL(locations []string, startTime, endTime float64) string {
	quoted := make([]string, 0, len(locations))
	for _, loc := range locations {
		quoted = append(quoted, "'"+loc+"'")
	}
	// Location is an interned id; join the location table to filter by component
	// name and surface the readable name under the original "Location" column,
	// rather than leaking the integer id (via t.*) or adding an extra column.
	whereClause := "loc.Locale IN (" + strings.Join(quoted, ",") + ")"
	timeClause := fmt.Sprintf(
		"t.StartTime >= %.15f AND t.EndTime <= %.15f", startTime, endTime)
	return `
SELECT
	t.ID,
	t.ParentID,
	t.Kind,
	t.What,
	loc.Locale AS Location,
	t.StartTime,
	t.EndTime
FROM trace t
JOIN location loc ON t.Location = loc.ID
WHERE ` + whereClause + `
AND ` + timeClause + fmt.Sprintf(`
ORDER BY t.StartTime, t.ID
LIMIT %d`, maxTraceContextRows)
}

func formatTraceRows(traceReader *SQLiteTraceReader, sqlStr string) string {
	rows, err := traceReader.Query(sqlStr)
	if err != nil {
		log.Println("Failed to query trace:", err)
		return ""
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		log.Println("Failed to get columns:", err)
		return ""
	}

	var b strings.Builder
	b.WriteString("[Reference Akita Trace File]\n")
	b.WriteString(strings.Join(columns, ","))
	b.WriteByte('\n')

	rowCount := 0
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range values {
			valuePtrs[i] = &values[i]
		}
		if err := rows.Scan(valuePtrs...); err != nil {
			log.Println("Failed to scan trace row:", err)
			continue
		}
		rowStrs := make([]string, 0, len(values))
		for _, val := range values {
			switch v := val.(type) {
			case nil:
				rowStrs = append(rowStrs, "")
			case []byte:
				rowStrs = append(rowStrs, string(v))
			case float64:
				rowStrs = append(rowStrs, fmt.Sprintf("%.9f", v))
			default:
				rowStrs = append(rowStrs, fmt.Sprintf("%v", v))
			}
		}
		b.WriteString(strings.Join(rowStrs, ","))
		b.WriteByte('\n')
		rowCount++
	}

	if rowCount >= maxTraceContextRows {
		b.WriteString(fmt.Sprintf(
			"[Note: trace truncated to the first %d events]\n", maxTraceContextRows))
	}
	b.WriteString("[End Akita Trace File]\n")
	return b.String()
}

func (s *Server) fetchTasksForMilestones(ctx context.Context, compName string, startTime, endTime float64) []Task {
	query := TaskQuery{
		Where:            compName,
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: true,
		EnableMilestones: true,
	}
	return s.traceReader.ListTasks(ctx, query)
}

func collectMilestoneKinds(tasks []Task) []string {
	kindSet := make(map[string]bool)
	for _, task := range tasks {
		for _, step := range task.Steps {
			kindSet[step.Kind] = true
		}
	}

	kinds := make([]string, 0, len(kindSet))
	for kind := range kindSet {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}

func generateStackedTimeData(tasks []Task, kinds []string, startTime, endTime float64, numDots int) []StackedTimeValue {
	data := make([]StackedTimeValue, 0, numDots)
	binDuration := (endTime - startTime) / float64(numDots)

	for i := 0; i < numDots; i++ {
		// Sample at the center of each bin and count, at that instant, how many
		// in-flight tasks are blocked by each reason.
		sampleTime := startTime + (float64(i)+0.5)*binDuration
		data = append(data, StackedTimeValue{
			Time:   sampleTime,
			Values: countBlockedTasksByKind(tasks, kinds, sampleTime),
		})
	}
	return data
}

// countBlockedTasksByKind counts, at sampleTime, how many in-flight tasks are
// blocked by each reason.
func countBlockedTasksByKind(tasks []Task, kinds []string, sampleTime float64) map[string]float64 {
	kindCounts := make(map[string]float64, len(kinds))
	for _, kind := range kinds {
		kindCounts[kind] = 0
	}

	for _, task := range tasks {
		if float64(task.StartTime) > sampleTime || float64(task.EndTime) < sampleTime {
			continue
		}
		if kind := findTaskBlockingKind(task, sampleTime); kind != "" {
			kindCounts[kind]++
		}
	}
	return kindCounts
}

// findTaskBlockingKind returns the reason the task is blocked on at time t: the
// kind of the first milestone at or after t (the next blocking reason to be
// released). A milestone marks the moment a reason is released, so the interval
// before it is time blocked on that reason. Returns "" when no milestone
// remains — the task is running to completion, not blocked.
func findTaskBlockingKind(task Task, t float64) string {
	best := ""
	bestTime := 0.0
	found := false

	for _, step := range task.Steps {
		stepTime := float64(step.Time)
		if stepTime >= t && (!found || stepTime < bestTime) {
			best = step.Kind
			bestTime = stepTime
			found = true
		}
	}

	return best
}

func (s *Server) calculateConcurrentTaskMilestones(
	ctx context.Context,
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
) *StackedComponentInfo {
	info := &StackedComponentInfo{
		Name:      compName,
		InfoType:  infoType,
		StartTime: startTime,
		EndTime:   endTime,
		Kinds:     []string{},
	}

	tasks := s.fetchTasksForMilestones(ctx, compName, startTime, endTime)
	info.Kinds = collectMilestoneKinds(tasks)
	info.Data = generateStackedTimeData(tasks, info.Kinds, startTime, endTime, numDots)

	return info
}
