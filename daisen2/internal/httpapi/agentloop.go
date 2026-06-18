package httpapi

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// This file implements Phase 2 of the DaisenBot agentic upgrade: a streamed,
// multi-step tool-calling loop with a guarded read-only `data_query` tool over
// the trace. See daisen2/docs/daisenbot-agent-plan.md §7.

// ---- Loop bounds (§4.6) ----

const (
	maxAgentIterations = 8                // model turns before we force a final answer
	maxAgentToolCalls  = 24               // total tool executions across the whole loop
	maxAgentWallClock  = 10 * time.Minute // overall budget for one agent run (vs. iterations × per-call ceiling)
)

// ---- Tool framework ----

// toolResult is what a tool returns: a text result (becomes the tool message)
// plus any images to attach as a follow-up multimodal user message — a tool
// message can't carry image content in the OpenAI schema.
type toolResult struct {
	text   string
	images []string // data URLs
}

// agentTool is a capability the model can invoke during the loop.
type agentTool struct {
	name        string
	description string
	parameters  map[string]interface{} // JSON Schema for the arguments
	run         func(ctx context.Context, args map[string]interface{}) (toolResult, error)
}

func (t agentTool) spec() map[string]interface{} {
	return map[string]interface{}{
		"type": "function",
		"function": map[string]interface{}{
			"name":        t.name,
			"description": t.description,
			"parameters":  t.parameters,
		},
	}
}

// ---- Streamed events (SSE payloads) ----

// agentEvent is one event streamed to the client during the loop.
type agentEvent struct {
	Type        string `json:"type"` // thinking | step | observation | message | error | done | render
	Tool        string `json:"tool,omitempty"`
	Args        string `json:"args,omitempty"`
	Observation string `json:"observation,omitempty"`
	Text        string `json:"text,omitempty"`
	Error       string `json:"error,omitempty"`
	// Type=="render": the backend asks the browser to capture an image and POST it
	// back to /api/agent/capture with this CaptureID. RenderKind is "screenshot"
	// (the current page) or "view" (render URL off-screen first).
	CaptureID  string `json:"captureId,omitempty"`
	RenderKind string `json:"renderKind,omitempty"`
	URL        string `json:"url,omitempty"`
}

type emitFunc func(agentEvent)

// ---- OpenAI chat-completions response shapes ----

type oaiToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type oaiResponse struct {
	Choices []struct {
		Message struct {
			Role      string        `json:"role"`
			Content   string        `json:"content"`
			ToolCalls []oaiToolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// callProvider makes one chat-completions call. When offerTools is true the tool
// specs are attached. It returns the assistant message map (to append to the
// conversation, preserving any tool_calls), the parsed tool calls, and the text.
func callProvider(
	ctx context.Context,
	cfg ProviderConfig,
	messages []map[string]interface{},
	toolSpecs []interface{},
	offerTools bool,
) (map[string]interface{}, []oaiToolCall, string, error) {
	payload := map[string]interface{}{
		"model":    cfg.Model,
		"messages": messages,
	}
	if cfg.Temperature != nil {
		payload["temperature"] = *cfg.Temperature
	}
	if offerTools && len(toolSpecs) > 0 {
		payload["tools"] = toolSpecs
		payload["tool_choice"] = "auto"
	}

	bodyBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, "", err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", cfg.BaseURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, nil, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", bearerToken(cfg.APIKey))
	}

	resp, err := guardedLLMClient.Do(req)
	if err != nil {
		return nil, nil, "", err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxChatResponseBytes))
	if resp.StatusCode != http.StatusOK {
		return nil, nil, "", fmt.Errorf("provider returned %d: %s",
			resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var parsed oaiResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, nil, "", fmt.Errorf("decoding provider response: %w", err)
	}
	if parsed.Error != nil {
		return nil, nil, "", fmt.Errorf("provider error: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return nil, nil, "", fmt.Errorf("provider returned no choices")
	}

	m := parsed.Choices[0].Message
	return assistantMessageFromOAI(m.Role, m.Content, m.ToolCalls), m.ToolCalls, m.Content, nil
}

// assistantMessageFromOAI converts a parsed OpenAI assistant message into the map
// we append to the conversation, preserving any tool_calls so the next request is
// well-formed.
func assistantMessageFromOAI(_ string, content string, toolCalls []oaiToolCall) map[string]interface{} {
	msg := map[string]interface{}{"role": "assistant"}
	if content != "" {
		msg["content"] = content
	}
	if len(toolCalls) > 0 {
		tcs := make([]interface{}, len(toolCalls))
		for i, tc := range toolCalls {
			tcs[i] = map[string]interface{}{
				"id":   tc.ID,
				"type": "function",
				"function": map[string]interface{}{
					"name":      tc.Function.Name,
					"arguments": tc.Function.Arguments,
				},
			}
		}
		msg["tool_calls"] = tcs
	}
	return msg
}

// runAgentLoop drives the multi-step tool-calling loop, emitting events as it
// goes. It is bounded by maxAgentIterations / maxAgentToolCalls and the ctx
// deadline; on exhaustion it makes one final no-tools call for a graceful answer.
func runAgentLoop(
	ctx context.Context,
	cfg ProviderConfig,
	messages []map[string]interface{},
	tools []agentTool,
	emit emitFunc,
) error {
	toolByName := make(map[string]agentTool, len(tools))
	specs := make([]interface{}, 0, len(tools))
	for _, t := range tools {
		toolByName[t.name] = t
		specs = append(specs, t.spec())
	}

	toolCallCount := 0
	for iter := 0; iter < maxAgentIterations; iter++ {
		offerTools := toolCallCount < maxAgentToolCalls

		msg, toolCalls, content, err := callProvider(ctx, cfg, messages, specs, offerTools)
		if err != nil && offerTools {
			// The endpoint may not support tools — retry once without them so a
			// non-tool model still answers (capability fallback).
			msg, toolCalls, content, err = callProvider(ctx, cfg, messages, nil, false)
		}
		if err != nil {
			return fmt.Errorf("agent loop failed on turn %d (%d tool calls so far): %w", iter+1, toolCallCount, err)
		}

		if len(toolCalls) == 0 {
			emit(agentEvent{Type: "message", Text: content})
			return nil
		}

		// The model often explains its reasoning alongside the tool calls — surface
		// that intermediate "thinking" so the user sees why each query is run.
		if strings.TrimSpace(content) != "" {
			emit(agentEvent{Type: "thinking", Text: content})
		}

		messages = append(messages, msg)
		messages = runToolCalls(ctx, toolByName, toolCalls, &toolCallCount, emit, messages)
	}

	// Iteration budget exhausted: force a final answer from the evidence so far.
	_, _, content, err := callProvider(ctx, cfg, messages, nil, false)
	if err != nil {
		return fmt.Errorf("agent loop failed on the final turn: %w", err)
	}
	emit(agentEvent{Type: "message", Text: content})
	return nil
}

// runToolCalls executes one assistant turn's batch of tool calls, appending the
// resulting messages to messages and returning the extended slice. Every tool_call
// in the assistant message must be answered by a tool message immediately after it,
// before any other role. So it appends ALL tool responses first, then attaches any
// captured images as a single follow-up user message — inserting the image user
// message between tool responses makes the provider reject the request (unanswered
// tool_call_id). toolCallCount is incremented in place as the budget is consumed.
func runToolCalls(
	ctx context.Context,
	toolByName map[string]agentTool,
	toolCalls []oaiToolCall,
	toolCallCount *int,
	emit emitFunc,
	messages []map[string]interface{},
) []map[string]interface{} {
	var imageParts []interface{}
	for _, tc := range toolCalls {
		// Respect the overall tool-call budget even within a single batched
		// turn (a model can emit many tool_calls at once). We must still answer
		// every tool_call_id with a tool message or the next provider request is
		// invalid, so reply with a stub instead of executing real work.
		if *toolCallCount >= maxAgentToolCalls {
			emit(agentEvent{Type: "observation", Tool: tc.Function.Name, Observation: "tool-call budget exhausted; skipped"})
			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      "Skipped: tool-call budget exhausted. Answer with the evidence gathered so far.",
			})
			continue
		}
		*toolCallCount++
		emit(agentEvent{Type: "step", Tool: tc.Function.Name, Args: tc.Function.Arguments})

		result := dispatchTool(ctx, toolByName, tc)
		emit(agentEvent{Type: "observation", Tool: tc.Function.Name, Observation: clip(result.text, 600)})
		messages = append(messages, map[string]interface{}{
			"role":         "tool",
			"tool_call_id": tc.ID,
			"content":      result.text,
		})
		for _, img := range result.images {
			imageParts = append(imageParts, map[string]interface{}{
				"type":      "image_url",
				"image_url": map[string]interface{}{"url": img},
			})
		}
	}
	// Images can't ride in a tool message (OpenAI schema), so attach them after
	// all tool responses as one multimodal user message (needs a vision model).
	if len(imageParts) > 0 {
		content := append([]interface{}{
			map[string]interface{}{"type": "text", "text": "Captured view(s):"},
		}, imageParts...)
		messages = append(messages, map[string]interface{}{"role": "user", "content": content})
	}
	return messages
}

func dispatchTool(ctx context.Context, byName map[string]agentTool, tc oaiToolCall) toolResult {
	tool, ok := byName[tc.Function.Name]
	if !ok {
		return toolResult{text: "Error: unknown tool " + tc.Function.Name}
	}
	var args map[string]interface{}
	if strings.TrimSpace(tc.Function.Arguments) != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return toolResult{text: "Error: could not parse tool arguments: " + err.Error()}
		}
	}
	out, err := tool.run(ctx, args)
	if err != nil {
		return toolResult{text: "Error: " + err.Error()}
	}
	return out
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// ---- data_query tool ----

const (
	dataQueryRowCap  = 1000
	dataQueryByteCap = 64 * 1024
	dataQueryTimeout = 15 * time.Second
)

const dataQueryDescription = `Run a single read-only SQL query (SELECT or WITH only) over the Akita trace
and get the rows back as CSV.

Schema:
- trace(ID, ParentID, Kind, What, Location, StartTime, EndTime) — one row per task/event. Location is an interned id.
- location(ID, Locale) — Locale is the human-readable component name; join trace.Location = location.ID.
- milestone(...) — sub-events attached to tasks (may be absent in some traces).

Notes: times are raw trace values; results are capped at 1000 rows. Prefer aggregates
(COUNT, AVG, MIN, MAX, GROUP BY) over dumping rows.`

func dataQueryTool(reader *SQLiteTraceReader) agentTool {
	return agentTool{
		name:        "data_query",
		description: dataQueryDescription,
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: what you are checking and why. Shown to the user as your reasoning for this step.",
				},
				"sql": map[string]interface{}{
					"type":        "string",
					"description": "A single read-only SQL SELECT/WITH statement over the trace schema.",
				},
			},
			"required": []string{"reason", "sql"},
		},
		run: func(ctx context.Context, args map[string]interface{}) (toolResult, error) {
			query, _ := args["sql"].(string)
			out, err := runDataQuery(ctx, reader, query)
			return toolResult{text: out}, err
		},
	}
}

var limitClausePattern = regexp.MustCompile(`(?i)\blimit\s+\d`)

// sanitizeReadonlySQL enforces single-statement SQL with a SELECT/WITH prefix and
// injects a row limit when none is present. This is a first-line filter; the
// authoritative read-only guarantee is the PRAGMA query_only connection in
// runDataQuery, since a prefix check alone can be bypassed by a write smuggled
// through a CTE (e.g. `WITH x AS (...) DELETE ... RETURNING`).
func sanitizeReadonlySQL(query string, rowCap int) (string, error) {
	q := strings.TrimSpace(query)
	q = strings.TrimRight(q, "; \t\n\r")
	if q == "" {
		return "", fmt.Errorf("empty query")
	}
	if strings.Contains(q, ";") {
		return "", fmt.Errorf("only a single statement is allowed")
	}
	upper := strings.ToUpper(strings.TrimLeft(q, "( \t\n\r"))
	if !strings.HasPrefix(upper, "SELECT") && !strings.HasPrefix(upper, "WITH") {
		return "", fmt.Errorf("only read-only SELECT/WITH queries are allowed")
	}
	if !limitClausePattern.MatchString(q) {
		q = fmt.Sprintf("%s LIMIT %d", q, rowCap)
	}
	return q, nil
}

func runDataQuery(ctx context.Context, reader *SQLiteTraceReader, query string) (string, error) {
	if reader == nil || reader.DB == nil {
		return "", fmt.Errorf("no trace is loaded")
	}
	safe, err := sanitizeReadonlySQL(query, dataQueryRowCap)
	if err != nil {
		return "", err
	}

	qctx, cancel := context.WithTimeout(ctx, dataQueryTimeout)
	defer cancel()

	// Engine-level read-only guard. The replay server opens a writable connection
	// (Init), and sanitizeReadonlySQL's SELECT/WITH prefix check can be bypassed by
	// a write smuggled through a CTE (e.g. `WITH x AS (...) DELETE ... RETURNING`).
	// PRAGMA query_only makes SQLite itself reject any write on this connection, so
	// model-influenced SQL can only observe the trace. Reset it before the
	// connection returns to the pool.
	conn, err := reader.DB.Conn(qctx)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(qctx, "PRAGMA query_only = ON"); err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer func() { _, _ = conn.ExecContext(qctx, "PRAGMA query_only = OFF") }()

	rows, err := conn.QueryContext(qctx, safe)
	if err != nil {
		return "", fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return formatRows(rows, dataQueryRowCap, dataQueryByteCap)
}

// formatRows serializes rows as CSV, hard-capped at rowCap rows and byteCap bytes
// so a broad query can never flood the model context.
func formatRows(rows *sql.Rows, rowCap, byteCap int) (string, error) {
	cols, err := rows.Columns()
	if err != nil {
		return "", err
	}

	var body strings.Builder
	body.WriteString(strings.Join(cols, ","))
	body.WriteString("\n")

	n := 0
	truncated := false
	for rows.Next() {
		if n >= rowCap {
			truncated = true
			break
		}
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return "", err
		}
		cells := make([]string, len(vals))
		for i, v := range vals {
			cells[i] = cellToString(v)
		}
		line := strings.Join(cells, ",")
		// Enforce the byte cap *before* writing the row. cellToString bounds a
		// single cell; this bounds the whole result, so an oversized row (e.g.
		// SELECT hex(zeroblob(1e8))) cannot blow past the cap into the LLM context.
		if body.Len()+len(line)+1 > byteCap {
			truncated = true
			break
		}
		body.WriteString(line)
		body.WriteString("\n")
		n++
	}
	if err := rows.Err(); err != nil {
		return "", err
	}

	summary := fmt.Sprintf("[%d rows]", n)
	if truncated {
		summary = fmt.Sprintf("[%d rows shown; result truncated — narrow the query or aggregate]", n)
	}
	return summary + "\n" + body.String(), nil
}

// dataQueryMaxCellBytes bounds a single cell so one huge value (a big blob/text
// column) is truncated rather than flooding the result and the model context.
const dataQueryMaxCellBytes = 4096

func cellToString(v interface{}) string {
	s := rawCellToString(v)
	if len(s) > dataQueryMaxCellBytes {
		return s[:dataQueryMaxCellBytes] + "…"
	}
	return s
}

func rawCellToString(v interface{}) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return strings.ReplaceAll(string(t), ",", " ")
	case float64:
		return fmt.Sprintf("%g", t)
	case string:
		return strings.ReplaceAll(t, ",", " ")
	default:
		return fmt.Sprintf("%v", t)
	}
}

// ---- Agent system prompt & message assembly ----

const agentSystemPrompt = `You are DaisenBot, an assistant that investigates Akita
computer-architecture simulation traces.

You can call the data_query tool to run read-only SQL over the trace. Use it to gather evidence
before answering questions about behavior, bottlenecks, or specific tasks. Every data_query call
must include a one-sentence "reason" describing what you are checking — it is shown to the user
as your reasoning, so make it clear.

You do NOT have the trace rows in your context. For any question about the trace's contents,
timings, counts, or behavior, you MUST gather the facts with data_query first — do not answer
such questions from memory or assumption, and never invent task IDs, durations, or counts. Run
at least one query before making a quantitative claim.

You can also SEE Daisen's visualizations:
- screenshot_current_view: capture what the user is currently looking at on screen, and look at it.
- daisen_view: render a specific Daisen view off-screen by its URL and look at it. URL scheme:
"/dashboard?widget=<component>&starttime=<t>&endtime=<t>&primary=<metric>&secondary=<metric>";
"/component?name=<component>&taskid=<id>&starttime=<t>&endtime=<t>";
"/task?id=<taskid>&where=<component>&kind=<kind>". Times are raw trace values.
Use these when a question is about visual patterns (timelines, bursts, periodicity, occupancy
shapes) that are easier to see than to query. Both take a one-sentence "reason".

Front door:
- If a question is a simple definition or can be answered from the provided context, answer directly without tools.
- If a question is ambiguous (e.g. which component, which time range), ask ONE concise clarifying question.
- Otherwise, investigate: form a hypothesis, run targeted data_query calls to confirm or refute it,
then answer with the evidence (cite the numbers you found). Prefer aggregates over dumping rows.

Common Akita bottleneck patterns to consider (seed list — not exhaustive): cache miss/thrashing,
queue backpressure / buffer-full stalls, limited outstanding requests (MSHRs), DRAM bank conflicts,
bandwidth saturation, head-of-line blocking, and address-translation (TLB) stalls.

Be concise and concrete. When you are uncertain, say so and report what you ruled out.`

// assembleAgentMessages builds the message list for an agent-mode request: just
// the agent system prompt followed by the user's conversation, verbatim. Nothing
// else is prepended — the agent fetches whatever trace data it needs with
// data_query rather than being handed context up front.
func assembleAgentMessages(body chatRequest) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0, len(body.Messages)+1)
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": agentSystemPrompt,
	})
	messages = append(messages, body.Messages...)
	return messages
}

// ---- SSE handler ----

func (s *Server) runAgentSSE(
	w http.ResponseWriter,
	r *http.Request,
	cfg ProviderConfig,
	messages []map[string]interface{},
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)

	emit := func(ev agentEvent) {
		data, err := json.Marshal(ev)
		if err != nil {
			return
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		if flusher != nil {
			flusher.Flush()
		}
	}

	capture := s.newCaptureRequester(emit)
	tools := []agentTool{
		dataQueryTool(s.traceReader),
		codeSearchTool(s.codeSource),
		codeReadTool(s.codeSource),
		daisenViewTool(capture),
		screenshotTool(capture),
	}
	// The replay server has no per-request deadline and the per-call ceiling is
	// per provider call, so a slow provider could otherwise hold this handler for
	// maxAgentIterations × that ceiling. Bound the whole run to a fixed budget.
	ctx, cancel := context.WithTimeout(r.Context(), maxAgentWallClock)
	defer cancel()
	if err := runAgentLoop(ctx, cfg, messages, tools, emit); err != nil {
		emit(agentEvent{Type: "error", Error: err.Error()})
	}
	emit(agentEvent{Type: "done"})
}

// ---- Viz perception: browser capture round-trip (Phase 5) ----
//
// The loop runs server-side, but rendering and capturing a view happen in the
// browser. A capture tool emits a "render" SSE event carrying a CaptureID, blocks
// until the browser POSTs the image to /api/agent/capture with that id, then
// returns the image as a multimodal observation.

const (
	captureTimeout  = 30 * time.Second
	maxCaptureBytes = 24 << 20 // 24 MiB image data URL
)

var captureCounter int64

func nextCaptureID() string {
	return fmt.Sprintf("cap-%d", atomic.AddInt64(&captureCounter, 1))
}

// captureRequester asks the browser to capture an image (kind "screenshot" for
// the current page, or "view" to render url off-screen first) and returns the
// resulting data URL.
type captureRequester func(ctx context.Context, kind, url string) (string, error)

func (s *Server) registerCapture(id string) chan string {
	ch := make(chan string, 1)
	s.capturesMu.Lock()
	if s.captures == nil {
		s.captures = make(map[string]chan string)
	}
	s.captures[id] = ch
	s.capturesMu.Unlock()
	return ch
}

func (s *Server) unregisterCapture(id string) {
	s.capturesMu.Lock()
	delete(s.captures, id)
	s.capturesMu.Unlock()
}

// resolveCapture delivers an image to a waiting capture request; returns false if
// none is waiting (already timed out / unknown id).
func (s *Server) resolveCapture(id, image string) bool {
	s.capturesMu.Lock()
	ch, ok := s.captures[id]
	if ok {
		delete(s.captures, id)
	}
	s.capturesMu.Unlock()
	if !ok {
		return false
	}
	ch <- image
	return true
}

func (s *Server) newCaptureRequester(emit emitFunc) captureRequester {
	return func(ctx context.Context, kind, url string) (string, error) {
		id := nextCaptureID()
		ch := s.registerCapture(id)
		defer s.unregisterCapture(id)

		emit(agentEvent{Type: "render", CaptureID: id, RenderKind: kind, URL: url})

		select {
		case img := <-ch:
			if strings.TrimSpace(img) == "" {
				return "", fmt.Errorf("the browser returned an empty capture")
			}
			return img, nil
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(captureTimeout):
			return "", fmt.Errorf("timed out waiting for the browser to capture the view")
		}
	}
}

// httpAgentCapture receives a captured image from the browser and hands it to the
// waiting tool, keyed by the capture id.
func (s *Server) httpAgentCapture(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ID    string `json:"id"`
		Image string `json:"image"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCaptureBytes)).Decode(&body); err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if body.ID == "" {
		http.Error(w, "missing id", http.StatusBadRequest)
		return
	}
	s.resolveCapture(body.ID, body.Image)
	w.WriteHeader(http.StatusNoContent)
}

func screenshotTool(capture captureRequester) agentTool {
	return agentTool{
		name: "screenshot_current_view",
		description: "Capture a screenshot of what the user is currently looking at on screen in Daisen, " +
			"and see it. Use this to ground your analysis in the user's current view.",
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: why you want to see the current screen. Shown to the user.",
				},
			},
			"required": []string{"reason"},
		},
		run: func(ctx context.Context, _ map[string]interface{}) (toolResult, error) {
			img, err := capture(ctx, "screenshot", "")
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{text: "Captured the current screen.", images: []string{img}}, nil
		},
	}
}

func daisenViewTool(capture captureRequester) agentTool {
	return agentTool{
		name: "daisen_view",
		description: "Render a specific Daisen view off-screen by its URL and see it as an image. Examples: " +
			"\"/component?name=L2Cache&starttime=0&endtime=379102000\", \"/dashboard?widget=L2Cache\", " +
			"\"/task?id=<taskid>\". Use it to inspect a chart or timeline you have not been shown.",
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"reason": map[string]interface{}{
					"type":        "string",
					"description": "One sentence: what you want to see and why. Shown to the user.",
				},
				"url": map[string]interface{}{
					"type":        "string",
					"description": "A Daisen path + query, e.g. /component?name=L2Cache&starttime=0&endtime=379102000",
				},
			},
			"required": []string{"reason", "url"},
		},
		run: func(ctx context.Context, args map[string]interface{}) (toolResult, error) {
			url, _ := args["url"].(string)
			if strings.TrimSpace(url) == "" {
				return toolResult{}, fmt.Errorf("a url is required")
			}
			img, err := capture(ctx, "view", url)
			if err != nil {
				return toolResult{}, err
			}
			return toolResult{text: "Rendered the view: " + url, images: []string{img}}, nil
		},
	}
}
