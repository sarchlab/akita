import assert from "node:assert/strict";
import test from "node:test";

import {
  storageKey,
  stripConversationImages,
  persistableConversations,
} from "../src/utils/conversationStore.mjs";

test("storageKey namespaces by trace id and falls back to default", () => {
  assert.equal(storageKey("akita_sim_abc"), "daisen.chat.akita_sim_abc");
  assert.equal(storageKey(""), "daisen.chat.default");
  assert.equal(storageKey(null), "daisen.chat.default");
  assert.equal(storageKey(undefined), "daisen.chat.default");
});

test("stripConversationImages drops captured step images, keeps everything else", () => {
  const input = [
    {
      id: "c1",
      title: "L2 latency",
      timestamp: 5,
      messages: [
        { role: "user", content: [{ type: "text", text: "why slow?" }] },
        {
          role: "assistant",
          content: [{ type: "text", text: "see here" }],
          steps: [
            { tool: "daisen_view", args: "{}", observation: "ok", image: "data:image/jpeg;base64,AAAA" },
            { thinking: "hmm" },
          ],
        },
      ],
    },
  ];

  const out = stripConversationImages(input);

  // image removed, but the rest of the step is intact.
  assert.equal(out[0].messages[1].steps[0].image, undefined);
  assert.equal(out[0].messages[1].steps[0].tool, "daisen_view");
  assert.equal(out[0].messages[1].steps[0].observation, "ok");
  assert.deepEqual(out[0].messages[1].steps[1], { thinking: "hmm" });
  // text content and metadata preserved.
  assert.equal(out[0].title, "L2 latency");
  assert.equal(out[0].messages[0].content[0].text, "why slow?");
  // input is not mutated (the original still carries the image).
  assert.equal(input[0].messages[1].steps[0].image, "data:image/jpeg;base64,AAAA");
});

test("stripConversationImages tolerates messages without steps", () => {
  const out = stripConversationImages([
    { id: "c", title: "t", timestamp: 1, messages: [{ role: "user", content: [] }] },
  ]);
  assert.deepEqual(out[0].messages[0], { role: "user", content: [] });
});

test("persistableConversations keeps only conversations with a user message", () => {
  const convos = [
    { id: "empty", title: "New Chat", timestamp: 1, messages: [] },
    { id: "assistantOnly", title: "x", timestamp: 2, messages: [{ role: "assistant", content: [] }] },
    { id: "real", title: "q", timestamp: 3, messages: [{ role: "user", content: [] }] },
  ];
  const kept = persistableConversations(convos);
  assert.deepEqual(kept.map((c) => c.id), ["real"]);
});
