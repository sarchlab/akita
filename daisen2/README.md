# Daisen

Daisen is the trace visualization tool for Akita simulations.

This README is a placeholder. Documentation for Daisen will be added here.

## Daisen Bot (chat assistant)

Daisen Bot answers questions about the trace you are viewing. It sends your
message — together with context from the selected components and the current
time range — to an LLM through the Daisen server.

### Configuring a model

There are two ways to provide an LLM endpoint; the in-app settings take
precedence over the server defaults.

1. **In the app (recommended).** Open the chat panel, click the gear icon, and
   choose a provider preset (OpenAI, OpenRouter, Groq, Ollama, or Custom), then
   enter the base URL, model, and API key. Any endpoint that implements the
   OpenAI `/chat/completions` API works, including local servers such as Ollama,
   LM Studio, and vLLM. The model field is backed by the provider's model list
   (fetched via the server from `{baseURL}/models`); use the refresh button to
   load it, then pick a model or type your own. By default the API key is kept
   in `sessionStorage` and cleared when the tab closes; tick **Remember key on
   this device** to persist it in `localStorage` instead.

2. **Server-side default (`.env`).** Create a `.env` next to the server to set a
   fallback used whenever the app does not supply its own values:

   ```
   OPENAI_URL="https://api.openai.com/v1/chat/completions"
   OPENAI_MODEL="gpt-4o"
   OPENAI_API_KEY="sk-proj-XXXXXXXXXXXX"
   ```

   The key may be given with or without a leading `Bearer `.

The API key sent from the app travels in the `X-LLM-Api-Key` request header
(not the request body) and is never persisted on the server.
