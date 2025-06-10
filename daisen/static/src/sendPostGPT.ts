
// export async function sendPostGPT(userInput: string, apiKey: string): Promise<string> {
//   const url = "https://ceyifan.openai.azure.com/openai/deployments/gpt-4o/chat/completions?api-version=2025-01-01-preview";
//   const headers = {
//     "Content-Type": "application/json",
//     "api-key": apiKey
//   };
//   const body = JSON.stringify({
//     messages: [
//       { role: "user", content: userInput }
//     ],
//     temperature: 0.7
//   });

//   try {
//     const response = await fetch(url, {
//       method: "POST",
//       headers,
//       body
//     });
//     if (!response.ok) {
//       throw new Error(`HTTP error! status: ${response.status}`);
//     }
//     const data = await response.json();
//     // Extract the assistant's message content
//     const gptContent = data?.choices?.[0]?.message?.content ?? "No response from GPT.";
//     return gptContent;
//   } catch (err) {
//     return `Error: ${(err as Error).message}`;
//   }
// }
export interface ChatMessage {
  role: "user" | "assistant" | "system";
  content: string;
}

export async function sendPostGPT(messages: ChatMessage[]): Promise<string> {
  const response = await fetch("/api/gpt", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ messages })
  });
  if (!response.ok) {
    return await response.text();
  }
  const data = await response.json();
  // Extract the assistant's message content
  return data?.choices?.[0]?.message?.content ?? "No response from GPT.";
}