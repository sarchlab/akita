import { useEffect, useState } from "react";

interface ResourceResponse {
  cpu_percent: number;
  memory_size: number;
}

export interface ResourcePoint extends ResourceResponse {
  timestamp: number;
}

export interface MinuteResourcePoint extends ResourcePoint {
  samples: number;
}

export interface ResourceHistory {
  seconds: ResourcePoint[];
  minutes: MinuteResourcePoint[];
}

export const RESOURCE_SAMPLE_INTERVAL_MS = 1000;
export const MAX_SECOND_SAMPLES = 60;
export const MAX_MINUTE_SAMPLES = 60;

let history: ResourceHistory = { seconds: [], minutes: [] };
let intervalID: number | null = null;
let collecting = false;
const listeners = new Set<() => void>();

function notifyResourceListeners() {
  listeners.forEach((listener) => listener());
}

async function collectResourceUsage() {
  if (collecting) {
    return;
  }

  collecting = true;
  try {
    const response = await fetch("/api/resource");
    const json: unknown = response.ok ? await response.json() : null;
    if (!json || typeof json !== "object") {
      return;
    }

    const resource = json as Partial<ResourceResponse>;
    const nextResources = {
      cpu_percent: typeof resource.cpu_percent === "number" ? resource.cpu_percent : 0,
      memory_size: typeof resource.memory_size === "number" ? resource.memory_size : 0,
    };
    const nextPoint = { ...nextResources, timestamp: Date.now() };
    const seconds = [...history.seconds, nextPoint].slice(-MAX_SECOND_SAMPLES);
    const minuteTimestamp = Math.floor(nextPoint.timestamp / 60000) * 60000;
    const lastMinute = history.minutes[history.minutes.length - 1];
    let minutes: MinuteResourcePoint[];

    if (lastMinute?.timestamp === minuteTimestamp) {
      const samples = lastMinute.samples + 1;
      const updatedMinute = {
        timestamp: minuteTimestamp,
        samples,
        cpu_percent:
          (lastMinute.cpu_percent * lastMinute.samples + nextPoint.cpu_percent) / samples,
        memory_size:
          (lastMinute.memory_size * lastMinute.samples + nextPoint.memory_size) / samples,
      };

      minutes = [...history.minutes.slice(0, -1), updatedMinute];
    } else {
      minutes = [...history.minutes, { ...nextResources, timestamp: minuteTimestamp, samples: 1 }];
    }

    history = { seconds, minutes: minutes.slice(-MAX_MINUTE_SAMPLES) };
    notifyResourceListeners();
  } catch {
    // Keep existing resource history if the endpoint is temporarily unavailable.
  } finally {
    collecting = false;
  }
}

export function ensureResourceUsageCollector() {
  if (intervalID !== null) {
    return;
  }

  void collectResourceUsage();
  intervalID = window.setInterval(collectResourceUsage, RESOURCE_SAMPLE_INTERVAL_MS);
}

export function ResourceUsageCollector() {
  useEffect(() => {
    ensureResourceUsageCollector();
  }, []);

  return null;
}

export function useResourceUsageHistory() {
  const [snapshot, setSnapshot] = useState<ResourceHistory>(() => {
    ensureResourceUsageCollector();
    return history;
  });

  useEffect(() => {
    ensureResourceUsageCollector();
    const listener = () => setSnapshot(history);
    listeners.add(listener);

    return () => {
      listeners.delete(listener);
    };
  }, []);

  return { history: snapshot };
}
