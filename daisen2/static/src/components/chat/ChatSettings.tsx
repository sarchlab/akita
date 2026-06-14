import { useCallback, useEffect, useState } from "react";
import { Check, Loader2, RefreshCw, X } from "lucide-react";
import { Button } from "../ui/button";
import { Input } from "../ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "../ui/select";
import { PROVIDER_PRESETS } from "../../hooks/useLLMSettings";
import type { LLMCapabilities, LLMSettings } from "../../types/chat";

interface ChatSettingsProps {
  settings: LLMSettings;
  update: (partial: Partial<LLMSettings>) => void;
  applyPreset: (presetId: string) => void;
  clearKey: () => void;
  capabilities: LLMCapabilities;
  onClose: () => void;
}

type TestResult = { ok: boolean; message: string };

export default function ChatSettings({
  settings,
  update,
  applyPreset,
  clearKey,
  capabilities,
  onClose,
}: ChatSettingsProps) {
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<TestResult | null>(null);
  const [models, setModels] = useState<string[]>([]);
  const [modelsLoading, setModelsLoading] = useState(false);
  const [modelsError, setModelsError] = useState<string | null>(null);
  const [modelMenuOpen, setModelMenuOpen] = useState(false);

  const loadModels = useCallback(async () => {
    const overrideEndpoint = settings.endpointConfigured || !capabilities.hasServerDefault;
    if (overrideEndpoint && !settings.baseURL.trim()) {
      setModelsError("Set a base URL first.");
      return;
    }
    setModelsLoading(true);
    setModelsError(null);
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (settings.apiKey.trim()) headers["X-Llm-Api-Key"] = settings.apiKey.trim();

      // When the user hasn't chosen an endpoint, list the server default's
      // models rather than forcing the UI's preset endpoint.
      const modelsBody: Record<string, unknown> = {};
      if (overrideEndpoint) {
        modelsBody.provider = settings.provider;
        modelsBody.baseURL = settings.baseURL;
      }

      const response = await fetch("/api/models", {
        method: "POST",
        headers,
        body: JSON.stringify(modelsBody),
      });
      if (!response.ok) {
        setModels([]);
        setModelsError((await response.text()).trim().slice(0, 300) || `HTTP ${response.status}`);
        return;
      }
      const json = (await response.json()) as { models?: string[] };
      setModels(Array.isArray(json.models) ? json.models : []);
    } catch (err) {
      setModelsError(err instanceof Error ? err.message : String(err));
    } finally {
      setModelsLoading(false);
    }
  }, [settings.apiKey, settings.baseURL, settings.provider, settings.endpointConfigured, capabilities.hasServerDefault]);

  // Load the model list when the panel opens or the provider preset changes, but
  // only once a key is available (most endpoints require one to list models) so
  // we don't surface a 401 before the user has entered anything. Manual Refresh
  // covers edits to a custom base URL or key without refetching on every key.
  useEffect(() => {
    if (settings.baseURL.trim() && settings.apiKey.trim()) void loadModels();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [settings.presetId]);

  async function testConnection() {
    setTesting(true);
    setTestResult(null);
    try {
      const headers: Record<string, string> = { "Content-Type": "application/json" };
      if (settings.apiKey.trim()) headers["X-Llm-Api-Key"] = settings.apiKey.trim();

      const requestBody: Record<string, unknown> = {
        messages: [{ role: "user", content: [{ type: "text", text: "ping" }] }],
        traceInfo: { selected: 0, startTime: 0, endTime: 0, selectedComponentNameList: [] },
        selectedGitHubRoutineKeys: [],
      };
      // Match the chat path: override the server's endpoint/model only when the
      // user picked one (or the server has no default), so Test connection
      // exercises the same endpoint a real chat would use.
      if (settings.endpointConfigured || !capabilities.hasServerDefault) {
        requestBody.provider = settings.provider;
        requestBody.baseURL = settings.baseURL;
        requestBody.model = settings.model;
      }

      const response = await fetch("/api/gpt", {
        method: "POST",
        headers,
        body: JSON.stringify(requestBody),
      });

      if (response.ok) {
        setTestResult({ ok: true, message: "Connection succeeded." });
      } else {
        const text = (await response.text()).trim();
        setTestResult({ ok: false, message: text.slice(0, 400) || `HTTP ${response.status}` });
      }
    } catch (err) {
      setTestResult({ ok: false, message: err instanceof Error ? err.message : String(err) });
    } finally {
      setTesting(false);
    }
  }

  // Show every model when the field is empty or holds an already-selected model
  // id; filter by substring only while the user is actively typing a partial.
  const modelQuery = settings.model.trim().toLowerCase();
  const modelExactlyMatches = models.some((model) => model.toLowerCase() === modelQuery);
  const filteredModels =
    modelQuery === "" || modelExactlyMatches
      ? models
      : models.filter((model) => model.toLowerCase().includes(modelQuery));

  return (
    <div className="min-h-0 flex-1 space-y-4 overflow-auto p-4 text-sm">
      <div className="flex items-center justify-between">
        <h3 className="font-semibold">Model & Provider</h3>
        <Button type="button" size="icon" variant="ghost" onClick={onClose} aria-label="Close settings">
          <X />
        </Button>
      </div>

      {capabilities.hasServerDefault ? (
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          The server has a default model configured
          {capabilities.defaultModel ? ` (${capabilities.defaultModel})` : ""}. Leave the key blank
          to use it, or override it below.
        </p>
      ) : (
        <p className="rounded-md bg-muted px-3 py-2 text-xs text-muted-foreground">
          No server-side model is configured. Enter a provider, model, and API key to use Daisen Bot.
        </p>
      )}

      <label className="block space-y-1">
        <span className="text-xs font-medium text-muted-foreground">Provider</span>
        <Select value={settings.presetId} onValueChange={applyPreset}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {PROVIDER_PRESETS.map((preset) => (
              <SelectItem key={preset.id} value={preset.id}>
                {preset.label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </label>

      <label className="block space-y-1">
        <span className="text-xs font-medium text-muted-foreground">Base URL (OpenAI-compatible)</span>
        <Input
          type="text"
          value={settings.baseURL}
          placeholder="https://api.openai.com/v1/chat/completions"
          onChange={(event) => update({ baseURL: event.target.value, presetId: "custom" })}
        />
      </label>

      <label className="block space-y-1">
        <span className="text-xs font-medium text-muted-foreground">Model</span>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Input
              type="text"
              autoComplete="off"
              value={settings.model}
              placeholder="gpt-4o"
              onChange={(event) => {
                update({ model: event.target.value, presetId: "custom" });
                setModelMenuOpen(true);
              }}
              onFocus={() => setModelMenuOpen(true)}
              onBlur={() => setModelMenuOpen(false)}
            />
            {modelMenuOpen && filteredModels.length > 0 ? (
              <ul className="absolute left-0 right-0 z-50 mt-1 max-h-60 overflow-auto rounded-md border bg-card text-card-foreground shadow-md">
                {/* onMouseDown (not onClick) so selection runs before the input's onBlur closes the menu. */}
                {filteredModels.map((model) => (
                  <li key={model}>
                    <button
                      type="button"
                      className="block w-full truncate px-2 py-1.5 text-left text-sm hover:bg-accent hover:text-accent-foreground"
                      onMouseDown={(event) => {
                        event.preventDefault();
                        update({ model, presetId: "custom" });
                        setModelMenuOpen(false);
                      }}
                    >
                      {model}
                    </button>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
          <Button
            type="button"
            variant="outline"
            size="icon"
            onClick={() => void loadModels()}
            disabled={modelsLoading}
            aria-label="Refresh model list"
          >
            {modelsLoading ? <Loader2 className="animate-spin" /> : <RefreshCw />}
          </Button>
        </div>
        {modelsError ? (
          <span className="text-xs text-destructive">{modelsError}</span>
        ) : models.length ? (
          <span className="text-xs text-muted-foreground">
            {models.length} models available — click to choose or type to filter.
          </span>
        ) : (
          <span className="text-xs text-muted-foreground">
            Type a model name, or refresh to load the list from the provider.
          </span>
        )}
      </label>

      <label className="block space-y-1">
        <span className="text-xs font-medium text-muted-foreground">API key</span>
        <div className="flex gap-2">
          <Input
            type="password"
            autoComplete="off"
            value={settings.apiKey}
            placeholder="sk-..."
            onChange={(event) => update({ apiKey: event.target.value })}
          />
          <Button type="button" variant="outline" size="sm" onClick={clearKey} disabled={!settings.apiKey}>
            Forget
          </Button>
        </div>
      </label>

      <label className="flex items-center gap-2">
        <input
          type="checkbox"
          checked={settings.remember}
          onChange={(event) => update({ remember: event.target.checked })}
        />
        <span className="text-xs text-muted-foreground">
          Remember key on this device (stores it in localStorage instead of clearing on tab close)
        </span>
      </label>

      <div className="flex items-center gap-2">
        <Button type="button" variant="outline" size="sm" onClick={() => void testConnection()} disabled={testing}>
          {testing ? <Loader2 className="animate-spin" /> : <Check />}
          Test connection
        </Button>
        {testResult ? (
          <span className={testResult.ok ? "text-xs text-emerald-600" : "text-xs text-destructive"}>
            {testResult.message}
          </span>
        ) : null}
      </div>
    </div>
  );
}
