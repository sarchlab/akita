// Persist DaisenBot conversations in the browser, keyed by trace, so the
// conversation selector survives reloads. Stored client-side only: the server
// holds no per-user state and the trace file is never mutated.
//
// Captured view images (step.image) are stripped before storing — they are large
// data URLs and re-render lazily from their view URL (see MessageBubble), so
// persisting them would blow the localStorage quota for no benefit.

const STORAGE_PREFIX = "daisen.chat.";

/** localStorage key for a trace's conversations. */
export function storageKey(traceId) {
  return STORAGE_PREFIX + (traceId || "default");
}

/**
 * Return conversations with captured step images removed. Pure — no storage
 * access — so it is unit-testable in node.
 */
export function stripConversationImages(conversations) {
  return conversations.map((chat) => ({
    ...chat,
    messages: (chat.messages ?? []).map((message) => {
      if (!message.steps) return message;
      return {
        ...message,
        steps: message.steps.map((step) => {
          if (!step.image) return step;
          const { image: _dropped, ...rest } = step;
          return rest;
        }),
      };
    }),
  }));
}

/** Only conversations with at least one user message are worth persisting. */
export function persistableConversations(conversations) {
  return conversations.filter((chat) =>
    (chat.messages ?? []).some((message) => message.role === "user"),
  );
}

/** Load a trace's conversations from localStorage; [] when none/unavailable. */
export function loadConversations(traceId) {
  try {
    const raw = window.localStorage.getItem(storageKey(traceId));
    if (!raw) return [];
    const parsed = JSON.parse(raw);
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

/** Persist a trace's conversations (images stripped, empty chats dropped). */
export function saveConversations(traceId, conversations) {
  try {
    const toStore = stripConversationImages(persistableConversations(conversations));
    window.localStorage.setItem(storageKey(traceId), JSON.stringify(toStore));
  } catch {
    // Quota exceeded or storage unavailable — conversations stay in memory.
  }
}
