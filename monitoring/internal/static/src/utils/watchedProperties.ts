export interface WatchedProperty {
  id: string;
  component: string;
  field: string;
  label: string;
  sampleKind: WatchedPropertySampleKind;
}

const STORAGE_KEY = "akita-monitoring2-watched-properties";
const CHANGE_EVENT = "akita-monitoring2-watched-properties-change";

export type WatchedPropertySampleKind = "value" | "count";

export function watchedPropertyID(component: string, field: string) {
  return `${encodeURIComponent(component)}|${encodeURIComponent(field)}`;
}

export function getWatchedProperties(): WatchedProperty[] {
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY);
    const parsed: unknown = raw ? JSON.parse(raw) : [];

    if (!Array.isArray(parsed)) {
      return [];
    }

    return parsed.filter(isWatchedProperty);
  } catch {
    return [];
  }
}

export function isPropertyWatched(component: string, field: string) {
  const id = watchedPropertyID(component, field);
  return getWatchedProperties().some((property) => property.id === id);
}

export function addWatchedProperty(
  component: string,
  field: string,
  sampleKind: WatchedPropertySampleKind,
  label = field,
) {
  const id = watchedPropertyID(component, field);
  const properties = getWatchedProperties();

  if (properties.some((property) => property.id === id)) {
    return;
  }

  writeWatchedProperties([...properties, { id, component, field, label, sampleKind }]);
}

export function removeWatchedProperty(component: string, field: string) {
  const id = watchedPropertyID(component, field);
  writeWatchedProperties(getWatchedProperties().filter((property) => property.id !== id));
}

export function subscribeToWatchedProperties(onChange: () => void) {
  window.addEventListener(CHANGE_EVENT, onChange);
  window.addEventListener("storage", onChange);

  return () => {
    window.removeEventListener(CHANGE_EVENT, onChange);
    window.removeEventListener("storage", onChange);
  };
}

function writeWatchedProperties(properties: WatchedProperty[]) {
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(properties));
  window.dispatchEvent(new Event(CHANGE_EVENT));
}

function isWatchedProperty(value: unknown): value is WatchedProperty {
  if (!value || typeof value !== "object") {
    return false;
  }

  const property = value as Partial<WatchedProperty>;
  return (
    typeof property.id === "string" &&
    typeof property.component === "string" &&
    typeof property.field === "string" &&
    typeof property.label === "string" &&
    isWatchedPropertySampleKind(property.sampleKind)
  );
}

function isWatchedPropertySampleKind(value: unknown): value is WatchedPropertySampleKind {
  return value === "value" || value === "count";
}
