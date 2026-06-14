export type UnitContent =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } };

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: UnitContent[];
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
  selectedGitHubRoutineKeys: string[];
  provider?: string;
  baseURL?: string;
  model?: string;
  temperature?: number;
}

export interface GPTResponse {
  content: string;
  totalTokens: number;
}

// LLMSettings is the user-configured provider connection. The API key is held
// alongside the non-secret fields here but persisted separately (see
// useLLMSettings) so it can follow a more conservative storage policy.
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
  // configured is true once the user has actually touched the provider settings.
  // Until then we send no endpoint/model overrides so the server's .env defaults
  // apply unchanged.
  configured: boolean;
}

export interface LLMCapabilities {
  hasServerDefault: boolean;
  defaultModel: string;
  defaultBaseURL: string;
  providers: string[];
}
