export type UnitContent =
  | { type: "text"; text: string }
  | { type: "image_url"; image_url: { url: string } };

export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: UnitContent[];
}

export interface GPTRequest {
  messages: ChatMessage[];
  traceInfo: TraceInformation;
  selectedGitHubRoutineKeys: string[];
}

export interface GPTResponse {
  content: string;
  totalTokens: number;
}

export interface GitHubIsAvailableResponse {
  available: number;
  routine_keys: string[];
}

export interface TraceInformation {
  selected: number;
  startTime: number;
  endTime: number;
  selectedComponentNameList: string[];
}

export interface UploadedFile {
  id: number;
  name: string;
  content: string;
  type: "file" | "image" | "image-screenshot";
  size: string;
}

export interface ChatConversation {
  id: string;
  title: string;
  messages: ChatMessage[];
  timestamp: number;
}
