package main

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strconv"

	"github.com/sarchlab/akita/v3/tracing"
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
		compInfo = calculateTimeWeightedTaskCount(
			compName, infoType,
			startTime, endTime, int(numDots),
			func(t tracing.Task) bool { return true },
			func(t tracing.Task) float64 { return float64(t.StartTime) },
			func(t tracing.Task) float64 { return float64(t.EndTime) },
		)
	case "BufferPressure":
		compInfo = calculateTimeWeightedTaskCount(
			compName, infoType,
			startTime, endTime, int(numDots),
			taskIsReqIn,
			func(t tracing.Task) float64 {
				return float64(t.ParentTask.StartTime)
			},
			func(t tracing.Task) float64 {
				return float64(t.StartTime)
			},
		)
	case "PendingReqOut":
		compInfo = calculateTimeWeightedTaskCount(
			compName, infoType,
			startTime, endTime, int(numDots),
			func(t tracing.Task) bool { return t.Kind == "req_out" },
			func(t tracing.Task) float64 { return float64(t.StartTime) },
			func(t tracing.Task) float64 { return float64(t.EndTime) },
		)
	default:
		log.Panicf("unknown info_type %s\n", infoType)
	}

	rsp, err := json.Marshal(compInfo)
	dieOnErr(err)

	_, err = w.Write(rsp)
	dieOnErr(err)
}

func taskIsReqIn(t tracing.Task) bool {
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

	query := tracing.TaskQuery{
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

	query := tracing.TaskQuery{
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

	query := tracing.TaskQuery{
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

type taskFilter func(t tracing.Task) bool
type taskTime func(t tracing.Task) float64

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

	query := tracing.TaskQuery{
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

func filterTask(tasks []tracing.Task, filter taskFilter) []tracing.Task {
	filteredTasks := []tracing.Task{}

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
	tasks []tracing.Task,
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
	tasks []tracing.Task,
	binStart, binEnd float64,
	taskStart, taskEnd taskTime,
) (tasksInBin []tracing.Task) {
	for _, t := range tasks {
		if isTaskOverlapsWithBin(t, binStart, binEnd, taskStart, taskEnd) {
			tasksInBin = append(tasksInBin, t)
		}
	}

	return tasksInBin
}

func isTaskOverlapsWithBin(
	t tracing.Task,
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

// func httpComponentReqTree(w http.ResponseWriter, r *http.Request) {
//     compName := r.FormValue("where")
//     compID := r.FormValue("id")
//     compWhat := r.FormValue("what")
//     startTime, err := strconv.ParseFloat(r.FormValue("start_time"), 64)
//     if err != nil {
//         http.Error(w, "Invalid start_time", http.StatusBadRequest)
//         return
//     }
//     endTime, err := strconv.ParseFloat(r.FormValue("end_time"), 64)
//     if err != nil {
//         http.Error(w, "Invalid end_time", http.StatusBadRequest)
//         return
//     }

//     query := tracing.TaskQuery{
//         EnableTimeRange: true,
//         StartTime:       startTime,
//         EndTime:         endTime,
//     }
//     allTasks := traceReader.ListTasks(query)

//     treeData := map[string]interface{}{
//         "id":        compID,
//         "type":      "component",
//         "what":      compWhat,
//         "where":     compName,
//         "startTime": startTime,
//         "endTime":   endTime,
//         "left":      make([]map[string]interface{}, 0),
//         "right":     make([]map[string]interface{}, 0),
//     }

//     leftComponents := make(map[string]bool)
//     rightComponents := make(map[string]bool)
//     taskMap := make(map[string]tracing.Task)

// 	relatedTasks := make([]tracing.Task, 0)
//     for _, task := range allTasks {
//         taskMap[task.ID] = task
//         if task.Where == compName || (task.ParentID != "" && taskMap[task.ParentID].Where == compName) {
//             relatedTasks = append(relatedTasks, task)
//         }
//     }
// 	for _, task := range relatedTasks {
// 		if task.Where == compName && task.ParentID != "" {
// 			parentTask, exists := taskMap[task.ParentID]
// 			if exists && parentTask.Where != compName {
// 				sourceComp := parentTask.Where
// 				key := sourceComp
// 				if !leftComponents[key] {
// 					leftComponents[key] = true
// 					treeData["left"] = append(treeData["left"].([]map[string]interface{}), map[string]interface{}{
// 						"id":    parentTask.ID,
// 						"type":  parentTask.Kind,
// 						"what":  parentTask.What,
// 						"where": sourceComp,
// 					})
// 				}
// 			}
// 		}

//         if task.Where == compName {
//             for _, potentialSubTask := range allTasks {
//                 if potentialSubTask.ParentID == task.ID && potentialSubTask.Where != compName {
//                     destComp := potentialSubTask.Where
//                     key := destComp + "|" + potentialSubTask.What
//                     if !rightComponents[key] {
//                         rightComponents[key] = true
//                         treeData["right"] = append(treeData["right"].([]map[string]interface{}), map[string]interface{}{
//                             "id":    potentialSubTask.ID,
//                             "type":  potentialSubTask.Kind,
//                             "what":  potentialSubTask.What,
//                             "where": destComp,
//                         })
//                     }
//                 }
//             }
//         }
//     }

//     rsp, err := json.Marshal(treeData)
//     if err != nil {
//         http.Error(w, err.Error(), http.StatusInternalServerError)
//         return
//     }

//     log.Printf("Response data: %s", string(rsp))
//     w.Header().Set("Content-Type", "application/json")
// 	_, err = w.Write(rsp)
// 	if err != nil {
// 		http.Error(w, "Failed to write response", http.StatusInternalServerError)
// 		return
// 	}
// }
func httpComponentReqTree(w http.ResponseWriter, r *http.Request) {
    compName, compID, compWhat, startTime, endTime, err := parseRequestParameters(r)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    allTasks := fetchAllTasks(startTime, endTime)

    taskMap := buildTaskMap(allTasks)

    relatedTasks := filterRelatedTasks(allTasks, taskMap, compName)

    treeData, err := buildTreeData(compName, compID, compWhat, startTime, endTime, relatedTasks, taskMap)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    sendJSONResponse(w, treeData)
}

func parseRequestParameters(r *http.Request) (compName, compID, compWhat string, startTime, endTime float64, err error) {
    compName = r.FormValue("where")
    compID = r.FormValue("id")
    compWhat = r.FormValue("what")

    startTime = strconv.ParseFloat(r.FormValue("start_time"))

    endTime = strconv.ParseFloat(r.FormValue("end_time"))

    return
}

func fetchAllTasks(startTime, endTime float64) []tracing.Task {
    query := tracing.TaskQuery{
        EnableTimeRange: true,
        StartTime:       startTime,
        EndTime:         endTime,
    }
    return traceReader.ListTasks(query)
}

func buildTaskMap(tasks []tracing.Task) map[string]tracing.Task {
    taskMap := make(map[string]tracing.Task)
    for _, task := range tasks {
        taskMap[task.ID] = task
    }
    return taskMap
}

func filterRelatedTasks(allTasks []tracing.Task, taskMap map[string]tracing.Task, compName string) []tracing.Task {
    relatedTasks := make([]tracing.Task, 0)
    for _, task := range allTasks {
        if task.Where == compName || (task.ParentID != "" && taskMap[task.ParentID].Where == compName) {
            relatedTasks = append(relatedTasks, task)
        }
    }
    return relatedTasks
}

func buildTreeData(compName, compID, compWhat string, startTime, endTime float64, relatedTasks []tracing.Task, taskMap map[string]tracing.Task) (map[string]interface{}, error) {
    treeData := map[string]interface{}{
        "id":        compID,
        "type":      "component",
        "what":      compWhat,
        "where":     compName,
        "startTime": startTime,
        "endTime":   endTime,
        "left":      make([]map[string]interface{}, 0),
        "right":     make([]map[string]interface{}, 0),
    }

    leftComponents := make(map[string]bool)
    rightComponents := make(map[string]bool)

    for _, task := range relatedTasks {
        if task.Where == compName && task.ParentID != "" {
            addLeftComponent(treeData, task, taskMap, leftComponents, compName)
        }
        if task.Where == compName {
            addRightComponents(treeData, task, taskMap, rightComponents, compName)
        }
    }

    return treeData, nil
}

func addLeftComponent(treeData map[string]interface{}, task tracing.Task, taskMap map[string]tracing.Task, leftComponents map[string]bool, compName string) {
    parentTask, exists := taskMap[task.ParentID]
    if exists && parentTask.Where != compName {
        sourceComp := parentTask.Where
        key := sourceComp
        if !leftComponents[key] {
            leftComponents[key] = true
            treeData["left"] = append(treeData["left"].([]map[string]interface{}), map[string]interface{}{
                "id":    parentTask.ID,
                "type":  parentTask.Kind,
                "what":  parentTask.What,
                "where": sourceComp,
            })
        }
    }
}

func addRightComponents(treeData map[string]interface{}, task tracing.Task, taskMap map[string]tracing.Task, rightComponents map[string]bool, compName string) {
    for _, potentialSubTask := range taskMap {
        if potentialSubTask.ParentID == task.ID && potentialSubTask.Where != compName {
            destComp := potentialSubTask.Where
            key := destComp + "|" + potentialSubTask.What
            if !rightComponents[key] {
                rightComponents[key] = true
                treeData["right"] = append(treeData["right"].([]map[string]interface{}), map[string]interface{}{
                    "id":    potentialSubTask.ID,
                    "type":  potentialSubTask.Kind,
                    "what":  potentialSubTask.What,
                    "where": destComp,
                })
            }
        }
    }
}

func sendJSONResponse(w http.ResponseWriter, data interface{}) {
    rsp, err := json.Marshal(data)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    log.Printf("Response data: %s", string(rsp))
    w.Header().Set("Content-Type", "application/json")
    _, err = w.Write(rsp)
    if err != nil {
        http.Error(w, "Failed to write response", http.StatusInternalServerError)
        return
    }
}
