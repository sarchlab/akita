import { useCallback, useEffect, useMemo, useState } from "react";
import type { LLMSettings } from "../types/chat";

// All presets speak the OpenAI-compatible protocol; the preset only fills in the
// base URL and a sensible default model. "custom" leaves both blank for the user.
export const PROVIDER_PRESETS = [
  {
    id: "openai",
    label: "OpenAI",
    baseURL: "https://api.openai.com/v1/chat/completions",
    model: "gpt-4o",
  },
  {
    id: "openrouter",
    label: "OpenRouter",
    baseURL: "https://openrouter.ai/api/v1/chat/completions",
    model: "openai/gpt-4o",
  },
  {
    id: "groq",
    label: "Groq",
    baseURL: "https://api.groq.com/openai/v1/chat/completions",
    model: "llama-3.3-70b-versatile",
  },
  {
    id: "ollama",
    label: "Ollama (local)",
    baseURL: "http://localhost:11434/v1/chat/completions",
    model: "llama3.1",
  },
  { id: "custom", label: "Custom…", baseURL: "", model: "" },
] as const;

const CONFIG_KEY = "daisen.llm.config";
const SECRET_KEY = "daisen.llm.apikey";

const DEFAULT_SETTINGS: LLMSettings = {
  provider: "openai-compatible",
  presetId: "openai",
  baseURL: "https://api.openai.com/v1/chat/completions",
  model: "gpt-4o",
  apiKey: "",
  remember: false,
};

function safeGet(storage: Storage, key: string): string | null {
  try {
    return storage.getItem(key);
  } catch {
    return null;
  }
}

function safeSet(storage: Storage, key: string, value: string) {
  try {
    storage.setItem(key, value);
  } catch {
    /* storage may be unavailable (private mode / quota); ignore */
  }
}

function safeRemove(storage: Storage, key: string) {
  try {
    storage.removeItem(key);
  } catch {
    /* ignore */
  }
}

function loadInitialSettings(): LLMSettings {
  const settings = { ...DEFAULT_SETTINGS };

  const rawConfig = safeGet(localStorage, CONFIG_KEY);
  if (rawConfig) {
    try {
      const parsed = JSON.parse(rawConfig) as Partial<LLMSettings>;
      Object.assign(settings, {
        provider: parsed.provider ?? settings.provider,
        presetId: parsed.presetId ?? settings.presetId,
        baseURL: parsed.baseURL ?? settings.baseURL,
        model: parsed.model ?? settings.model,
        remember: parsed.remember ?? settings.remember,
      });
    } catch {
      /* corrupt config; fall back to defaults */
    }
  }

  // A key in localStorage means the user opted to remember it; otherwise it
  // lives in sessionStorage and is cleared when the tab closes.
  const persistedKey = safeGet(localStorage, SECRET_KEY);
  if (persistedKey !== null) {
    settings.apiKey = persistedKey;
    settings.remember = true;
  } else {
    settings.apiKey = safeGet(sessionStorage, SECRET_KEY) ?? "";
  }

  return settings;
}

export function useLLMSettings() {
  const [settings, setSettings] = useState<LLMSettings>(loadInitialSettings);

  // Persist non-secret fields in localStorage; route the key to the storage that
  // matches the user's "remember" choice and clear it from the other one.
  useEffect(() => {
    const { apiKey, ...nonSecret } = settings;
    safeSet(localStorage, CONFIG_KEY, JSON.stringify(nonSecret));

    if (settings.remember) {
      safeSet(localStorage, SECRET_KEY, apiKey);
      safeRemove(sessionStorage, SECRET_KEY);
    } else {
      safeSet(sessionStorage, SECRET_KEY, apiKey);
      safeRemove(localStorage, SECRET_KEY);
    }
  }, [settings]);

  const update = useCallback((partial: Partial<LLMSettings>) => {
    setSettings((current) => ({ ...current, ...partial }));
  }, []);

  const applyPreset = useCallback((presetId: string) => {
    const preset = PROVIDER_PRESETS.find((entry) => entry.id === presetId);
    if (!preset) return;
    setSettings((current) =>
      preset.id === "custom"
        ? { ...current, presetId }
        : { ...current, presetId, baseURL: preset.baseURL, model: preset.model },
    );
  }, []);

  const clearKey = useCallback(() => {
    setSettings((current) => ({ ...current, apiKey: "" }));
    safeRemove(localStorage, SECRET_KEY);
    safeRemove(sessionStorage, SECRET_KEY);
  }, []);

  const isConfigured = settings.baseURL.trim() !== "" && settings.model.trim() !== "";

  return useMemo(
    () => ({ settings, update, applyPreset, clearKey, isConfigured }),
    [settings, update, applyPreset, clearKey, isConfigured],
  );
}
