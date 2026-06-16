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
	"time"
)

// This file implements Phase 2 of the DaisenBot agentic upgrade: a streamed,
// multi-step tool-calling loop with a guarded read-only `data_query` tool over
// the trace. See daisen2/docs/daisenbot-agent-plan.md §7.

// ---- Loop bounds (§4.6) ----

const (
	maxAgentIterations = 8  // model turns before we force a final answer
	maxAgentToolCalls  = 24 // total tool executions across the whole loop
)

// ---- Tool framework ----

// agentTool is a capability the model can invoke during the loop.
type agentTool struct {
	name        string
	description string
	parameters  map[string]interface{} // JSON Schema for the arguments
	run         func(ctx context.Context, args map[string]interface{}) (string, error)
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
	Type        string `json:"type"` // step | observation | message | error | done
	Tool        string `json:"tool,omitempty"`
	Args        string `json:"args,omitempty"`
	Observation string `json:"observation,omitempty"`
	Text        string `json:"text,omitempty"`
	Error       string `json:"error,omitempty"`
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
	msg := map[string]interface{}{"role": "assistant"}
	if m.Content != "" {
		msg["content"] = m.Content
	}
	if len(m.ToolCalls) > 0 {
		tcs := make([]interface{}, len(m.ToolCalls))
		for i, tc := range m.ToolCalls {
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
	return msg, m.ToolCalls, m.Content, nil
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
			return err
		}

		if len(toolCalls) == 0 {
			emit(agentEvent{Type: "message", Text: content})
			return nil
		}

		messages = append(messages, msg)
		for _, tc := range toolCalls {
			toolCallCount++
			emit(agentEvent{Type: "step", Tool: tc.Function.Name, Args: tc.Function.Arguments})

			result := dispatchTool(ctx, toolByName, tc)
			emit(agentEvent{Type: "observation", Tool: tc.Function.Name, Observation: clip(result, 600)})
			messages = append(messages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": tc.ID,
				"content":      result,
			})
		}
	}

	// Iteration budget exhausted: force a final answer from the evidence so far.
	_, _, content, err := callProvider(ctx, cfg, messages, nil, false)
	if err != nil {
		return err
	}
	emit(agentEvent{Type: "message", Text: content})
	return nil
}

func dispatchTool(ctx context.Context, byName map[string]agentTool, tc oaiToolCall) string {
	tool, ok := byName[tc.Function.Name]
	if !ok {
		return "Error: unknown tool " + tc.Function.Name
	}
	var args map[string]interface{}
	if strings.TrimSpace(tc.Function.Arguments) != "" {
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			return "Error: could not parse tool arguments: " + err.Error()
		}
	}
	out, err := tool.run(ctx, args)
	if err != nil {
		return "Error: " + err.Error()
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

const dataQueryDescription = `Run a single read-only SQL query (SELECT or WITH only) over the Akita trace and get the rows back as CSV.

Schema:
- trace(ID, ParentID, Kind, What, Location, StartTime, EndTime) — one row per task/event. Location is an interned id.
- location(ID, Locale) — Locale is the human-readable component name; join trace.Location = location.ID.
- milestone(...) — sub-events attached to tasks (may be absent in some traces).

Notes: times are raw trace values; results are capped at 1000 rows. Prefer aggregates (COUNT, AVG, MIN, MAX, GROUP BY) over dumping rows.`

func dataQueryTool(reader *SQLiteTraceReader) agentTool {
	return agentTool{
		name:        "data_query",
		description: dataQueryDescription,
		parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"sql": map[string]interface{}{
					"type":        "string",
					"description": "A single read-only SQL SELECT/WITH statement over the trace schema.",
				},
			},
			"required": []string{"sql"},
		},
		run: func(ctx context.Context, args map[string]interface{}) (string, error) {
			query, _ := args["sql"].(string)
			return runDataQuery(ctx, reader, query)
		},
	}
}

var limitClausePattern = regexp.MustCompile(`(?i)\blimit\s+\d`)

// sanitizeReadonlySQL enforces single-statement read-only SQL and injects a row
// limit when none is present. Requiring a SELECT/WITH prefix with no embedded
// statement separator structurally blocks writes, PRAGMA, ATTACH, and DDL.
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

	rows, err := reader.QueryContext(qctx, safe)
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
		if n >= rowCap || body.Len() >= byteCap {
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
		body.WriteString(strings.Join(cells, ","))
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

func cellToString(v interface{}) string {
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

const agentSystemPrompt = `You are DaisenBot, an assistant that investigates Akita computer-architecture simulation traces.

You can call the data_query tool to run read-only SQL over the trace. Use it to gather evidence before answering questions about behavior, bottlenecks, or specific tasks.

Front door:
- If a question is a simple definition or can be answered from the provided context, answer directly without tools.
- If a question is ambiguous (e.g. which component, which time range), ask ONE concise clarifying question.
- Otherwise, investigate: form a hypothesis, run targeted data_query calls to confirm or refute it, then answer with the evidence (cite the numbers you found). Prefer aggregates over dumping rows.

Common Akita bottleneck patterns to consider (seed list — not exhaustive): cache miss/thrashing, queue backpressure / buffer-full stalls, limited outstanding requests (MSHRs), DRAM bank conflicts, bandwidth saturation, head-of-line blocking, and address-translation (TLB) stalls.

Be concise and concrete. When you are uncertain, say so and report what you ruled out.`

// assembleAgentMessages builds the message list for an agent-mode request: the
// agent system prompt, then the user's conversation with the trace-context CSV
// prepended to the latest message (same context the single-shot path uses).
func assembleAgentMessages(body chatRequest, reader *SQLiteTraceReader) []map[string]interface{} {
	traceHeader := buildAkitaTraceHeader(reader, body.TraceInfo)

	messages := make([]map[string]interface{}, 0, len(body.Messages)+1)
	messages = append(messages, map[string]interface{}{
		"role":    "system",
		"content": agentSystemPrompt,
	})

	user := body.Messages
	if traceHeader != "" && len(user) > 0 {
		if contentArr, ok := user[len(user)-1]["content"].([]interface{}); ok && len(contentArr) > 0 {
			if firstContent, ok := contentArr[0].(map[string]interface{}); ok {
				firstText, _ := firstContent["text"].(string)
				firstContent["text"] = traceHeader + firstText
			}
		}
	}
	messages = append(messages, user...)
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

	tools := []agentTool{dataQueryTool(s.traceReader)}
	if err := runAgentLoop(r.Context(), cfg, messages, tools, emit); err != nil {
		emit(agentEvent{Type: "error", Error: err.Error()})
	}
	emit(agentEvent{Type: "done"})
}
