import { useCallback, useState } from "react";
import type { LLMSettings } from "../types/chat";

type ModelListSettings = Pick<LLMSettings, "provider" | "baseURL" | "apiKey">;

interface ModelListResult {
  ok: boolean;
  models: string[];
  message: string;
}

export async function fetchModelList(settings: ModelListSettings): Promise<ModelListResult> {
  if (!settings.baseURL.trim()) {
    return { ok: false, models: [], message: "Set a base URL first." };
  }

  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (settings.apiKey.trim()) headers["X-Llm-Api-Key"] = settings.apiKey.trim();

  const response = await fetch("/api/models", {
    method: "POST",
    headers,
    body: JSON.stringify({ provider: settings.provider, baseURL: settings.baseURL }),
  });

  if (!response.ok) {
    const text = (await response.text()).trim().slice(0, 400);
    return { ok: false, models: [], message: text || `HTTP ${response.status}` };
  }

  const json = (await response.json()) as { models?: string[] };
  return {
    ok: true,
    models: Array.isArray(json.models) ? json.models : [],
    message: "Connection succeeded.",
  };
}

export function useModelList(settings: ModelListSettings) {
  const [models, setModels] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const loadModels = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await fetchModelList(settings);
      setModels(result.models);
      setError(result.ok ? null : result.message.slice(0, 300));
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setLoading(false);
    }
  }, [settings]);

  return { models, loading, error, loadModels };
}
