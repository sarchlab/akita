package httpapi

import _ "embed"

// agentSystemPrompt is DaisenBot's system prompt. It is authored as markdown in
// prompts/agent_system.md and embedded at build time, so editing the prompt — the
// tool guidance, the front door, the source map, the bottleneck catalog — is a
// markdown edit with no Go change. assembleAgentMessages prepends it as the system
// message of every agent request.
//
//go:embed prompts/agent_system.md
var agentSystemPrompt string
