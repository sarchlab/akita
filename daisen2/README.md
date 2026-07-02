# Daisen

Daisen is the trace visualization tool for Akita simulations.

This README is a placeholder. Documentation for Daisen will be added here.

## Daisen Bot (chat assistant)

Daisen Bot answers questions about the trace you are viewing. It sends your
message — together with context from the selected components and the current
time range — to an LLM through the Daisen server.

### Configuring a model

The LLM provider is configured entirely in the browser — **the server stores no
credentials.** Open the chat panel, click the gear icon, and choose a provider
preset (OpenAI, OpenRouter, Groq, Ollama, or Custom), then enter the base URL,
model, and API key. Any endpoint that implements the OpenAI `/chat/completions`
API works, including local servers such as Ollama, LM Studio, and vLLM (leave
the key blank for keyless local servers).

The model field is backed by the provider's model list (fetched via the server
from `{baseURL}/models`); use the refresh button to load it, then pick a model
or type your own.

The API key is sent on each request in the `X-Llm-Api-Key` header (not the body)
and is never written to disk on the server. In the browser it is kept in
`sessionStorage` and cleared when the tab closes; tick **Remember key on this
device** to persist it in `localStorage` instead.

### Reaching internal model servers

To stop the server from being used to reach internal services (SSRF), base URLs
that resolve to private, loopback, or link-local addresses are rejected by
default, and direct connections are pinned to the validated address. When
running Daisen for yourself with a local model server (Ollama, LM Studio, vLLM),
set `DAISEN_ALLOW_PRIVATE_LLM_URL=1` to allow them.

Outbound requests honor the standard `HTTP_PROXY`/`HTTPS_PROXY` environment
variables. When a request is routed through a proxy, the proxy performs the final
DNS resolution and connection, so egress filtering for proxied requests is
enforced by the proxy — point Daisen at a proxy you trust.
