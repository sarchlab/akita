export type UnitContent =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } };

// AgentStep is one tool invocation in the agent loop (Phase 2), shown as the
// visible "thinking" trail under an assistant message.
export interface AgentStep {
  tool: string;
  args?: string;
  observation?: string;
}

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: UnitContent[];
  steps?: AgentStep[];
}

export interface UploadedFile {
  id: number;
  name: string;
  content: string;
  type: "file" | "image" | "image-screenshot";
  size: string;
}

export interface TraceInformation {
  selected: number;
  startTime: number;
  endTime: number;
  selectedComponentNameList: string[];
}

export interface GPTRequest {
  messages: ChatMessage[];
  traceInfo: TraceInformation;
  provider?: string;
  baseURL?: string;
  model?: string;
  temperature?: number;
}

export interface GPTResponse {
  content: string;
  totalTokens: number;
}

// LLMSettings is the user-configured provider connection. The LLM config lives
// entirely in the browser — the server holds no credentials. The API key is
// persisted separately from the non-secret fields (see useLLMSettings) so it can
// follow a more conservative storage policy.
export interface LLMSettings {
  // provider is the wire protocol. Only "openai-compatible" is supported today.
  provider: string;
  // presetId tracks which UI preset is selected (or "custom").
  presetId: string;
  baseURL: string;
  model: string;
  apiKey: string;
  // remember persists the API key to localStorage instead of sessionStorage.
  remember: boolean;
}
