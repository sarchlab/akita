import { useEffect, useState } from "react";

// The loaded trace's stable id, used to scope browser-stored conversations to the
// trace being viewed (see conversationStore). Returns null until resolved, then a
// string. Falls back to "default" so persistence still works if the endpoint is
// unavailable (older server, dev, etc.).
export function useTraceId() {
  const [traceId, setTraceId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    (async () => {
      let id = "default";
      try {
        const response = await fetch("/api/trace_info");
        if (response.ok) {
          const body: { traceId?: string } = await response.json();
          if (body.traceId) id = body.traceId;
        }
      } catch {
        // Keep the "default" fallback so conversations still persist.
      }
      if (!cancelled) setTraceId(id);
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  return traceId;
}
