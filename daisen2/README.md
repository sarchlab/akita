# Daisen

Daisen is Akita's trace visualization and analysis tool. It serves a SQLite
trace through a Go HTTP API and a React UI for exploring simulation time ranges,
component residency, task hierarchies, topology, resource blocking, recorded
source, and Daisen Bot's trace-aware assistant.

## Running Daisen

Build an Akita simulation with tracing enabled, then point Daisen at the
generated SQLite trace:

```sh
go run ./daisen2/cmd/daisen2 -trace path/to/trace.sqlite -addr :3001
```

The server listens on the supplied address and serves both the API and the
bundled UI. During frontend development, run the Vite dev server from
`daisen2/static`; it proxies `/api` to `localhost:3001`.

```sh
cd daisen2/static
npm install
npm run dev
```

For a production frontend bundle:

```sh
cd daisen2/static
npm run build
```

## What To Inspect

- Dashboard: component-level task density and metric charts.
- Component view: a scoped task timeline, hierarchy, and blocking reasons.
- Task view: ancestors, descendants, milestones, and selected-task details.
- Resource view: tasks blocked on a hardware resource over a time window.
- Topology and code browser: recorded structure and source when present in the
  trace.

## Daisen Bot (chat assistant)

Daisen Bot answers questions about the trace you are viewing. The browser sends
your message to the Daisen server, and the server runs a bounded tool-using loop
that can query trace data, read recorded source, and request view captures when
the selected model supports those capabilities.

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
