import { useEffect, useState } from "react";

export type Mode = "live" | "replay";

interface ModeState {
  mode: Mode | null;
  loading: boolean;
  error: string | null;
}

function normalizeMode(candidate: unknown): Mode | null {
  if (typeof candidate !== "string") {
    return null;
  }

  const normalized = candidate.trim().toLowerCase();

  if (normalized === "live" || normalized === "replay") {
    return normalized;
  }

  return null;
}

/**
 * Parses /api/mode response payload and normalizes to supported mode literals.
 *
 * Primary contract is JSON object: {"mode":"live"|"replay"}.
 * We also tolerate plain-text fallback payloads ("live" / "replay").
 */
export function parseModeResponse(payloadText: string): Mode | null {
  const trimmedPayload = payloadText.trim();

  if (trimmedPayload.length === 0) {
    return null;
  }

  try {
    const parsedPayload = JSON.parse(trimmedPayload) as unknown;

    const parsedAsMode = normalizeMode(parsedPayload);
    if (parsedAsMode) {
      return parsedAsMode;
    }

    if (
      parsedPayload !== null &&
      typeof parsedPayload === "object" &&
      "mode" in parsedPayload
    ) {
      return normalizeMode((parsedPayload as { mode?: unknown }).mode);
    }
  } catch {
    // Tolerate legacy plain-text mode responses.
  }

  return normalizeMode(trimmedPayload);
}

export function useMode(): ModeState {
  const [mode, setMode] = useState<Mode | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const controller = new AbortController();

    fetch("/api/mode", { signal: controller.signal })
      .then((res) => {
        if (!res.ok) {
          throw new Error(`HTTP ${res.status}`);
        }
        return res.text();
      })
      .then((text) => {
        const parsedMode = parseModeResponse(text);

        if (!parsedMode) {
          throw new Error("Invalid /api/mode response payload");
        }

        setMode(parsedMode);
        setLoading(false);
      })
      .catch((err: unknown) => {
        if (err instanceof DOMException && err.name === "AbortError") return;
        setError(err instanceof Error ? err.message : String(err));
        setLoading(false);
      });

    return () => controller.abort();
  }, []);

  return { mode, loading, error };
}
