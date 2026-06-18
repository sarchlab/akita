package httpapi

import (
	"strings"
	"testing"
)

func TestAgentSystemPromptEmbedded(t *testing.T) {
	if len(agentSystemPrompt) < 500 {
		t.Fatalf("embedded agent system prompt looks too short (%d bytes) — //go:embed wiring broken?",
			len(agentSystemPrompt))
	}

	// The prompt must document every tool the agent is offered so the model knows
	// they exist and when to reach for them.
	for _, want := range []string{
		"DaisenBot", "data_query", "code_search", "code_read",
		"daisen_view", "screenshot_current_view",
	} {
		if !strings.Contains(agentSystemPrompt, want) {
			t.Errorf("system prompt does not mention %q", want)
		}
	}
}
