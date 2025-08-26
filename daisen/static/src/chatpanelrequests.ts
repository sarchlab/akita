export type UnitContent =
  | { type: "text"; text: string }           // typeValue is text
  | { type: "image_url"; image_url: { url: string } };     // typeValue is image URL


export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: UnitContent[];
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

export interface GPTRequest {
  messages: ChatMessage[];
  traceInfo: TraceInformation;
  selectedGitHubRoutineKeys: string[];
}

export async function sendPostGPT(request: GPTRequest): Promise<GPTResponse> {
  const response = await fetch("/api/gpt", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(request)
  });
  if (!response.ok) {
    return {
      content: await response.text(),
      totalTokens: -1
    };
  }
  const data = await response.json();
  return {
    content: data?.choices?.[0]?.message?.content ?? "No response from GPT.",
    totalTokens: typeof data?.usage?.total_tokens === "number" ? data.usage.total_tokens : -1
  };
}

export async function sendGetGitHubIsAvailable(): Promise<GitHubIsAvailableResponse> {
  const response = await fetch("/api/githubisavailable", {
    method: "GET",
    headers: { "Content-Type": "application/json" }
  });
  // console.log("Response from /api/githubisavailable:", response);
  const data = await response.json();
  // console.log("Response from /api/githubisavailable:", data);
  return data;
}

export async function sendGetCheckEnvFile(): Promise<{ exists: boolean }> {
  try {
    const response = await fetch("/api/checkenv", {
      method: "GET",
      headers: { "Content-Type": "application/json" }
    });
    if (!response.ok) {
      console.error("Failed to check .env file:", response.status);
      return { exists: false };
    }
    return await response.json();
  } catch (error) {
    console.error("Error checking .env file:", error);
    return { exists: false };
  }
}