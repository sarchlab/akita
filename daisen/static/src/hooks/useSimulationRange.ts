import { useEffect, useState } from "react";
import type { Task } from "../types/task";
import { useRenderReady } from "./useRenderReady";

export function useSimulationRange() {
  const [range, setRange] = useState({ startTime: 0, endTime: 0.000001 });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;

    async function loadRange() {
      try {
        const simulationResponse = await fetch("/api/trace?kind=Simulation");
        if (simulationResponse.ok) {
          const tasks: Task[] = await simulationResponse.json();
          const simulation = Array.isArray(tasks) ? tasks[0] : null;
          if (simulation) {
            return { startTime: simulation.start_time, endTime: simulation.end_time };
          }
        }
      } catch {
        // Fall back to trace table bounds below.
      }

      try {
        const rangeResponse = await fetch("/api/trace_range");
        if (!rangeResponse.ok) {
          return null;
        }
        const traceRange: { start_time?: number; end_time?: number } = await rangeResponse.json();
        if (
          typeof traceRange.start_time === "number" &&
          typeof traceRange.end_time === "number" &&
          traceRange.start_time < traceRange.end_time
        ) {
          return { startTime: traceRange.start_time, endTime: traceRange.end_time };
        }
      } catch {
        return null;
      }

      return null;
    }

    loadRange()
      .then((nextRange) => {
        if (!cancelled && nextRange) {
          setRange(nextRange);
        }
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useRenderReady(loading);

  return { ...range, loading };
}
