import { useEffect, useState } from "react";
import type { LLMCapabilities } from "../types/chat";

const EMPTY: LLMCapabilities = {
  hasServerDefault: false,
  defaultModel: "",
  defaultBaseURL: "",
  providers: ["openai-compatible"],
};

// useLLMCapabilities reports what the server can do on its own (i.e. whether a
// .env default exists), so the UI knows whether the user must supply a key.
export function useLLMCapabilities() {
  const [capabilities, setCapabilities] = useState<LLMCapabilities>(EMPTY);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let active = true;
    fetch("/api/llm-capabilities")
      .then((response) => (response.ok ? response.json() : EMPTY))
      .then((json: Partial<LLMCapabilities>) => {
        if (!active) return;
        setCapabilities({ ...EMPTY, ...json });
      })
      .catch(() => active && setCapabilities(EMPTY))
      .finally(() => active && setLoading(false));
    return () => {
      active = false;
    };
  }, []);

  return { capabilities, loading };
}
