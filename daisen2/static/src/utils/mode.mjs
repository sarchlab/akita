const normalizeMode = (candidate) => {
  if (typeof candidate !== "string") {
    return null;
  }

  const normalized = candidate.trim().toLowerCase();

  if (normalized === "live" || normalized === "replay") {
    return normalized;
  }

  return null;
};

export function parseModeResponse(payloadText) {
  const trimmedPayload = payloadText.trim();

  if (trimmedPayload.length === 0) {
    return null;
  }

  try {
    const parsedPayload = JSON.parse(trimmedPayload);

    const parsedAsMode = normalizeMode(parsedPayload);
    if (parsedAsMode) {
      return parsedAsMode;
    }

    if (
      parsedPayload !== null &&
      typeof parsedPayload === "object" &&
      "mode" in parsedPayload
    ) {
      return normalizeMode(parsedPayload.mode);
    }
  } catch {
    // Tolerate legacy plain-text mode responses.
  }

  return normalizeMode(trimmedPayload);
}
