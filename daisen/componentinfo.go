package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type TimeValue struct {
	Time  float64 `json:"time"`
	Value float64 `json:"value"`
}

type ComponentInfo struct {
	Name      string      `json:"name"`
	InfoType  string      `json:"info_type"`
	StartTime float64     `json:"start_time"`
	EndTime   float64     `json:"end_time"`
	Data      []TimeValue `json:"data"`
}

func httpComponentNames(w http.ResponseWriter, r *http.Request) {
	componentNames := traceReader.ListComponents()

	rsp, err := json.Marshal(componentNames)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func httpComponentInfo(w http.ResponseWriter, r *http.Request) {
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
		compInfo = calculateReqIn(
			compName, startTime, endTime, int(numDots))
	case "ReqCompleteCount":
		compInfo = calculateReqComplete(
			compName, startTime, endTime, int(numDots))
	case "AvgLatency":
		compInfo = calculateAvgLatency(
			compName, startTime, endTime, int(numDots))
	case "ConcurrentTask":
		compInfo = calculateConcurrentTask(
			compInfo, compName, infoType, startTime, endTime, numDots)
	case "BufferPressure":
		compInfo = calculateBufferPressure(
			compInfo, compName, infoType, startTime, endTime, numDots)
	case "PendingReqOut":
		compInfo = calculatePendingReqOut(
			compInfo, compName, infoType, startTime, endTime, numDots)
	default:
		log.Panicf("unknown info_type %s\n", infoType)
	}

	rsp, err := json.Marshal(compInfo)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func calculateConcurrentTask(
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = calculateTimeWeightedTaskCount(
		compName, infoType,
		startTime, endTime, int(numDots),
		func(t Task) bool { return true },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func calculateBufferPressure(
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = calculateTimeWeightedTaskCount(
		compName, infoType,
		startTime, endTime, int(numDots),
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

func calculatePendingReqOut(
	compInfo *ComponentInfo,
	compName, infoType string,
	startTime, endTime float64,
	numDots int64,
) *ComponentInfo {
	compInfo = calculateTimeWeightedTaskCount(
		compName, infoType,
		startTime, endTime, int(numDots),
		func(t Task) bool { return t.Kind == "req_out" },
		func(t Task) float64 { return float64(t.StartTime) },
		func(t Task) float64 { return float64(t.EndTime) },
	)

	return compInfo
}

func taskIsReqIn(t Task) bool {
	return t.Kind == "req_in" && t.ParentTask != nil
}

func calculateReqIn(
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

	query := TaskQuery{
		Where:            compName,
		Kind:             "req_in",
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: true,
	}
	reqs := traceReader.ListTasks(query)

	totalDuration := endTime - startTime
	binDuration := totalDuration / float64(numDots)

	for i := 0; i < numDots; i++ {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		reqCount := 0

		for _, r := range reqs {
			if float64(r.StartTime) > binStartTime &&
				float64(r.StartTime) < binEndTime {
				reqCount++
			}
		}

		tv := TimeValue{
			Time:  binStartTime + 0.5*binDuration,
			Value: float64(reqCount) / binDuration,
		}

		info.Data = append(info.Data, tv)
	}

	return info
}

func calculateReqComplete(
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

	query := TaskQuery{
		Where:            compName,
		Kind:             "req_in",
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: true,
	}
	reqs := traceReader.ListTasks(query)

	totalDuration := endTime - startTime
	binDuration := totalDuration / float64(numDots)

	for i := 0; i < numDots; i++ {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		reqCount := 0

		for _, r := range reqs {
			if float64(r.EndTime) > binStartTime &&
				float64(r.EndTime) < binEndTime {
				reqCount++
			}
		}

		tv := TimeValue{
			Time:  binStartTime + 0.5*binDuration,
			Value: float64(reqCount) / binDuration,
		}

		info.Data = append(info.Data, tv)
	}

	return info
}

func calculateAvgLatency(
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

	query := TaskQuery{
		Where:            compName,
		Kind:             "req_in",
		EnableTimeRange:  true,
		StartTime:        startTime,
		EndTime:          endTime,
		EnableParentTask: true,
	}
	reqs := traceReader.ListTasks(query)

	totalDuration := endTime - startTime
	binDuration := totalDuration / float64(numDots)

	for i := 0; i < numDots; i++ {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		sum := 0.0
		reqCount := 0

		for _, r := range reqs {
			if float64(r.EndTime) > binStartTime &&
				float64(r.EndTime) < binEndTime {
				sum += float64(r.EndTime - r.StartTime)
				reqCount++
			}
		}

		value := 0.0
		if reqCount > 0 {
			value = sum / float64(reqCount)
		}

		tv := TimeValue{
			Time:  binStartTime + 0.5*binDuration,
			Value: value,
		}

		info.Data = append(info.Data, tv)
	}

	return info
}

type timestamp struct {
	time    float64
	isStart bool
}

type timestamps []timestamp

func (ts timestamps) Len() int {
	return len(ts)
}

func (ts timestamps) Less(i, j int) bool {
	return ts[i].time < ts[j].time
}

func (ts timestamps) Swap(i, j int) {
	ts[i], ts[j] = ts[j], ts[i]
}

type taskFilter func(t Task) bool
type taskTime func(t Task) float64

func calculateTimeWeightedTaskCount(
	compName, infoType string,
	startTime, endTime float64,
	numDots int,
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
		EnableParentTask: true,
	}
	tasks := traceReader.ListTasks(query)
	tasks = filterTask(tasks, filter)

	totalDuration := endTime - startTime
	binDuration := totalDuration / float64(numDots)

	for i := 0; i < numDots; i++ {
		binStartTime := float64(i)*binDuration + startTime
		binEndTime := float64(i+1)*binDuration + startTime

		tasksInBin := getTasksInBin(
			tasks,
			binStartTime, binEndTime,
			increaseTime, decreaseTime,
		)
		timestamps := taskToTimeStamps(tasksInBin, increaseTime, decreaseTime)
		avgCount := calculateAvgTaskCount(
			timestamps, binStartTime, binEndTime)

		tv := TimeValue{
			Time:  binStartTime + 0.5*binDuration,
			Value: avgCount,
		}

		info.Data = append(info.Data, tv)
	}

	return info
}

func filterTask(tasks []Task, filter taskFilter) []Task {
	filteredTasks := []Task{}

	for _, t := range tasks {
		if filter(t) {
			filteredTasks = append(filteredTasks, t)
		}
	}

	return filteredTasks
}

func calculateAvgTaskCount(
	timestamps timestamps,
	binStartTime, binEndTime float64,
) float64 {
	var count int
	var timeByCount float64

	prevTime := binStartTime

	for _, ts := range timestamps {
		if ts.time < binStartTime {
			if ts.isStart {
				count++
			} else {
				count--
			}

			continue
		} else if ts.time >= binEndTime {
			break
		} else {
			duration := ts.time - prevTime
			if duration < 0 {
				panic("duration is smaller than 0")
			}
			timeByCount += duration * float64(count)
			prevTime = ts.time

			if ts.isStart {
				count++
			} else {
				count--
			}
		}
	}

	duration := binEndTime - prevTime
	timeByCount += duration * float64(count)

	avgCount := timeByCount / (binEndTime - binStartTime)

	return avgCount
}

func taskToTimeStamps(
	tasks []Task,
	taskStart, taskEnd taskTime,
) []timestamp {
	timestampList := make(timestamps, 0, len(tasks)*2)

	for _, t := range tasks {
		timestampStart := timestamp{
			time:    taskStart(t),
			isStart: true,
		}

		timestampEnd := timestamp{
			time: taskEnd(t),
		}

		timestampList = append(timestampList, timestampStart, timestampEnd)
	}

	sort.Sort(timestampList)

	return timestampList
}

func getTasksInBin(
	tasks []Task,
	binStart, binEnd float64,
	taskStart, taskEnd taskTime,
) (tasksInBin []Task) {
	for _, t := range tasks {
		if isTaskOverlapsWithBin(t, binStart, binEnd, taskStart, taskEnd) {
			tasksInBin = append(tasksInBin, t)
		}
	}

	return tasksInBin
}

func isTaskOverlapsWithBin(
	t Task,
	binStart, binEnd float64,
	taskStart, taskEnd taskTime,
) bool {
	if taskEnd(t) < binStart {
		return false
	}

	if taskStart(t) > binEnd {
		return false
	}

	return true
}

// Add this helper function outside buildOpenAIPayload:
func buildAkitaTraceHeader(traceReader *SQLiteTraceReader, traceInfo map[string]interface{}) string {
	selected, _ := traceInfo["selected"].(float64)
	if selected == 0 {
		return ""
	}
	startTime, _ := traceInfo["startTime"].(float64)
	endTime, _ := traceInfo["endTime"].(float64)
	selectedComponentNameList, _ := traceInfo["selectedComponentNameList"].([]interface{})
	locations := extractLocations(selectedComponentNameList)
	if len(locations) == 0 {
		return ""
	}
	sqlStr := buildTraceSQL(locations, startTime, endTime)
	return formatTraceRows(traceReader, sqlStr)
}

func extractLocations(selectedComponentNameList []interface{}) []string {
	locations := make([]string, 0, len(selectedComponentNameList))
	for _, v := range selectedComponentNameList {
		if s, ok := v.(string); ok {
			locations = append(locations, s)
		}
	}
	return locations
}

func buildTraceSQL(locations []string, startTime, endTime float64) string {
	quoted := make([]string, 0, len(locations))
	for _, loc := range locations {
		quoted = append(quoted, "'"+loc+"'")
	}
	whereClause := "Location IN (" + strings.Join(quoted, ",") + ")"
	timeClause := fmt.Sprintf("StartTime >= %.15f AND EndTime <= %.15f", startTime, endTime)
	return `
SELECT *
FROM trace
WHERE ` + whereClause + `
AND ` + timeClause
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

	header := "[Reference Akita Trace File]\n" + strings.Join(columns, ",") + "\n"
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
		header += strings.Join(rowStrs, ",") + "\n"
	}
	header += "[End Akita Trace File]\n"
	return header
}

func buildOpenAIPayload(
	ctx context.Context,
	model string,
	messages []map[string]interface{},
	traceInfo map[string]interface{},
	selectedGitHubRoutineKeys []string,
) ([]byte, error) {
	combinedTraceHeader := buildAkitaTraceHeader(traceReader, traceInfo)
	routineFile := "componentgithubroutine.json"
	data, err := os.ReadFile(routineFile)
	if err != nil {
		log.Println("Failed to read routine file:", err)
		return nil, err
	}
	var routineMap map[string][]string
	if err := json.Unmarshal(data, &routineMap); err != nil {
		log.Println("Failed to unmarshal routine file:", err)
		return nil, err
	}

	urlSet := make(map[string]struct{})
	for _, key := range selectedGitHubRoutineKeys {
		if urls, ok := routineMap[key]; ok {
			for _, u := range urls {
				urlSet[u] = struct{}{}
			}
		}
	}
	// Flatten, deduplicate, and sort
	urlList := make([]string, 0, len(urlSet))
	for u := range urlSet {
		urlList = append(urlList, u)
	}
	sort.Strings(urlList)

	combinedRepoHeader := ""
	for _, url := range urlList {
		content := httpGithubRaw(ctx, url)
		if content == "" {
			continue
		}
		fileName := url
		if idx := strings.Index(url, "sarchlab/"); idx != -1 {
			fileName = url[idx:]
		}
		combinedRepoHeader += "[Reference File " + fileName + "]\n"
		combinedRepoHeader += content + "\n"
		combinedRepoHeader += "[End " + fileName + "]\n"
	}

	if len(messages) > 0 {
		if contentArr, ok := messages[len(messages)-1]["content"].([]interface{}); ok && len(contentArr) > 0 {
			if firstContent, ok := contentArr[0].(map[string]interface{}); ok {
				firstText, _ := firstContent["text"].(string)
				firstContent["text"] = combinedTraceHeader + combinedRepoHeader + firstText
			}
		}
	}

	// detect whether to add the deforehand prompt
	if len(messages) == 0 || messages[0]["role"] != "system" {
		loadedTextBytes, err := os.ReadFile("beforehandprompt.txt")
		if err != nil {
			log.Println("Failed to read beforehandprompt.txt:", err)
			return nil, err
		}
		loadedText := string(loadedTextBytes)
		systemMsg := map[string]interface{}{
			"role": "system",
			"content": []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": loadedText,
				},
			},
		}
		// Prepend systemMsg to messages
		messages = append([]map[string]interface{}{systemMsg}, messages...)
	}

	payload := map[string]interface{}{
		"model":       model,
		"messages":    messages,
		"temperature": 0.7,
	}
	return json.Marshal(payload)
}

func sendOpenAIRequest(ctx context.Context, apiKey, url string, payloadBytes []byte) (*http.Response, error) {
	openaiReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(payloadBytes))
	if err != nil {
		return nil, err
	}
	openaiReq.Header.Set("Content-Type", "application/json")
	openaiReq.Header.Set("Authorization", apiKey)
	return http.DefaultClient.Do(openaiReq)
}

func httpGithubRaw(ctx context.Context, url string) string {
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	client := &http.Client{}
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		log.Println("Failed to create GitHub raw request:", err)
		return ""
	}
	if githubPAT != "" {
		req.Header.Set("Authorization", githubPAT)
	}
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		log.Printf("Failed to fetch raw GitHub file: %s, status: %d, err: %v\n", url, resp.StatusCode, err)
		return ""
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Failed to read GitHub raw response body:", err)
		return ""
	}
	return string(body)
}

func httpGPTProxy(w http.ResponseWriter, r *http.Request) {
	_ = godotenv.Load(".env")
	openaiApiKey := os.Getenv("OPENAI_API_KEY")
	openaiURL := os.Getenv("OPENAI_URL")
	openaiModel := os.Getenv("OPENAI_MODEL")
	if openaiApiKey == "" || openaiURL == "" || openaiModel == "" {
		http.Error(
			w,
			"[Error: \".env\" not found or OpenAI-related variable missing] "+
				"Please create or update file "+
				"\"akita/daisen/.env\" and write these contents (example):\n"+
				"```\n"+
				"OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n"+
				"OPENAI_MODEL=\"gpt-4o\"\n"+
				"OPENAI_API_KEY=\"Bearer sk-proj-XXXXXXXXXXXX\"\n"+
				"GITHUB_PERSONAL_ACCESS_TOKEN=\"Bearer ghp_XXXXXXXXXXXX\"\n"+
				"```\n",
			http.StatusInternalServerError,
		)
		return
	}
	var req struct {
		Messages                  []map[string]interface{} `json:"messages"`
		TraceInfo                 map[string]interface{}   `json:"traceInfo"`
		SelectedGitHubRoutineKeys []string                 `json:"selectedGitHubRoutineKeys"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	payloadBytes, err := buildOpenAIPayload(
		r.Context(), openaiModel, req.Messages, req.TraceInfo, req.SelectedGitHubRoutineKeys)
	if err != nil {
		http.Error(w, "Failed to marshal payload: "+err.Error(), http.StatusInternalServerError)
		return
	}

	resp, err := sendOpenAIRequest(r.Context(), openaiApiKey, openaiURL, payloadBytes)
	if err != nil {
		http.Error(
			w,
			"Failed to contact OpenAI: "+err.Error(),
			http.StatusBadGateway,
		)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	w.Write(body)
}

func httpGithubIsAvailableProxy(w http.ResponseWriter, r *http.Request) {
	_ = godotenv.Load(".env")
	githubPAT := os.Getenv("GITHUB_PERSONAL_ACCESS_TOKEN")
	if githubPAT == "" {
		http.Error(
			w,
			"[Error: \".env\" not found or GitHub-related variable missing] "+
				"Please create or update file "+
				"\"akita/daisen/.env\" and write these contents (example):\n"+
				"```\n"+
				"OPENAI_URL=\"https://api.openai.com/v1/chat/completions\"\n"+
				"OPENAI_MODEL=\"gpt-4o\"\n"+
				"OPENAI_API_KEY=\"Bearer sk-proj-XXXXXXXXXXXX\"\n"+
				"GITHUB_PERSONAL_ACCESS_TOKEN=\"Bearer ghp_XXXXXXXXXXXX\"\n"+
				"```\n",
			http.StatusInternalServerError,
		)
		return
	}

	client := &http.Client{}
	req, err := http.NewRequestWithContext(r.Context(), "GET", "https://api.github.com/user", nil)
	if err != nil {
		http.Error(w, "Failed to create GitHub request: "+err.Error(), http.StatusInternalServerError)
		return
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", githubPAT)

	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != 200 {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]interface{}{
			"available":    0,
			"routine_keys": []string{},
		}); err != nil {
			http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}
	defer resp.Body.Close()
	// Read routine keys from componentgithubroutine.json
	routineKeys := []string{}
	routineFile := "componentgithubroutine.json"
	data, err := os.ReadFile(routineFile)
	if err == nil {
		var routineMap map[string]interface{}
		if err := json.Unmarshal(data, &routineMap); err == nil {
			for k := range routineMap {
				routineKeys = append(routineKeys, k)
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"available":    1,
		"routine_keys": routineKeys,
	}); err != nil {
		http.Error(w, "Failed to encode JSON: "+err.Error(), http.StatusInternalServerError)
		return
	}
}
