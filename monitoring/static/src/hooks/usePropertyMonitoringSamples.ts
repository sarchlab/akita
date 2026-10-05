import { useEffect, useState } from "react";
import {
  getWatchedProperties,
  subscribeToWatchedProperties,
  type WatchedProperty,
} from "../utils/watchedProperties";

interface SethNode {
  k: number;
  v?: unknown;
  l?: number;
}

interface SethSnapshot {
  r: string;
  dict: Record<string, SethNode>;
}

export interface PropertySample {
  timePs: number;
  value: number;
}

export type PropertySamples = Record<string, PropertySample[]>;

export const MAX_PROPERTY_SAMPLES = 120;
export const PROPERTY_SAMPLE_INTERVAL_MS = 1000;

let samples: PropertySamples = {};
let properties: WatchedProperty[] = [];
let intervalID: number | null = null;
let collecting = false;
let unsubscribeWatchedProperties: (() => void) | null = null;
const listeners = new Set<() => void>();

function fieldRequestPath(componentName: string, fieldName: string) {
  return `/api/field/${encodeURIComponent(
    JSON.stringify({ comp_name: componentName, field_name: fieldName }),
  )}`;
}

function watchedSnapshotValue(snapshot: SethSnapshot, sampleKind: WatchedProperty["sampleKind"]) {
  const node = snapshot.dict[snapshot.r];
  if (!node) {
    return null;
  }

  if (sampleKind === "count") {
    if (node.k === 0) {
      return 0;
    }

    if (typeof node.l === "number" && Number.isFinite(node.l)) {
      return node.l;
    }

    if (Array.isArray(node.v)) {
      return node.v.length;
    }

    if (node.v && typeof node.v === "object") {
      return Object.keys(node.v).length;
    }

    return null;
  }

  const value = node.v;

  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  return null;
}

async function fetchEngineTime() {
  const response = await fetch("/api/now");
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  const json = (await response.json()) as { now?: unknown };
  return typeof json.now === "number" ? json.now : Date.now() * 1_000_000_000;
}

async function fetchWatchedPropertyValue(property: WatchedProperty) {
  const response = await fetch(fieldRequestPath(property.component, property.field));
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }

  return watchedSnapshotValue((await response.json()) as SethSnapshot, property.sampleKind);
}

function notifySampleListeners() {
  listeners.forEach((listener) => listener());
}

function trimRemovedProperties() {
  const propertyIDs = new Set(properties.map((property) => property.id));
  const nextSamples = Object.fromEntries(
    Object.entries(samples).filter(([id]) => propertyIDs.has(id)),
  );

  if (Object.keys(nextSamples).length !== Object.keys(samples).length) {
    samples = nextSamples;
    notifySampleListeners();
  }
}

function refreshWatchedProperties() {
  properties = getWatchedProperties();
  trimRemovedProperties();
}

async function collectPropertySamples() {
  if (collecting || !properties.length) {
    return;
  }

  collecting = true;
  try {
    const timePs = await fetchEngineTime();
    const values = await Promise.all(
      properties.map(async (property) => ({
        property,
        value: await fetchWatchedPropertyValue(property),
      })),
    );

    const nextSamples = { ...samples };
    let changed = false;

    values.forEach(({ property, value }) => {
      if (value === null) {
        return;
      }

      nextSamples[property.id] = [...(nextSamples[property.id] ?? []), { timePs, value }].slice(
        -MAX_PROPERTY_SAMPLES,
      );
      changed = true;
    });

    if (changed) {
      samples = nextSamples;
      notifySampleListeners();
    }
  } catch {
    // Keep existing samples if a component is temporarily unavailable.
  } finally {
    collecting = false;
  }
}

export function ensurePropertyMonitoringCollector() {
  if (!unsubscribeWatchedProperties) {
    refreshWatchedProperties();
    unsubscribeWatchedProperties = subscribeToWatchedProperties(() => {
      refreshWatchedProperties();
      void collectPropertySamples();
    });
  }

  if (intervalID === null) {
    void collectPropertySamples();
    intervalID = window.setInterval(collectPropertySamples, PROPERTY_SAMPLE_INTERVAL_MS);
  }
}

export function PropertyMonitoringCollector() {
  useEffect(() => {
    ensurePropertyMonitoringCollector();
  }, []);

  return null;
}

export function usePropertyMonitoringSamples() {
  const [snapshot, setSnapshot] = useState<PropertySamples>(() => {
    ensurePropertyMonitoringCollector();
    return samples;
  });

  useEffect(() => {
    ensurePropertyMonitoringCollector();
    const listener = () => setSnapshot(samples);
    listeners.add(listener);

    return () => {
      listeners.delete(listener);
    };
  }, []);

  return snapshot;
}
