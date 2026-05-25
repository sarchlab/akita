import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("React chat panel uses the shared markdown and upload policy helpers", async () => {
  const source = await readFile(new URL("../src/components/chat/ChatPanel.tsx", import.meta.url), "utf8");
  const bubbleSource = await readFile(new URL("../src/components/chat/MessageBubble.tsx", import.meta.url), "utf8");

  assert.match(source, /FILE_UPLOAD_ACCEPT/);
  assert.match(source, /IMAGE_UPLOAD_ACCEPT/);
  assert.match(source, /validateUploadedFile/);
  assert.match(source, /isImageUploadCandidate/);
  assert.match(bubbleSource, /renderChatMarkdown/);
  assert.match(bubbleSource, /renderMathInElement/);
});

test("React simulation range hook falls back to trace table bounds", async () => {
  const source = await readFile(new URL("../src/hooks/useSimulationRange.ts", import.meta.url), "utf8");

  assert.match(source, /\/api\/trace\?kind=Simulation/);
  assert.match(source, /\/api\/trace_range/);
});

test("React frontend does not ship the removed standalone graph page", async () => {
  const packageSource = await readFile(new URL("../package.json", import.meta.url), "utf8");
  const viteSource = await readFile(new URL("../vite.config.mjs", import.meta.url), "utf8");
  const chatSource = await readFile(new URL("../src/components/chat/ChatPanel.tsx", import.meta.url), "utf8");
  const appSource = await readFile(new URL("../src/App.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(packageSource, /chart\.js/);
  assert.doesNotMatch(viteSource, /datavisualization/);
  assert.doesNotMatch(chatSource, /datavisualization/);
  assert.doesNotMatch(appSource, /\/live/);
});
