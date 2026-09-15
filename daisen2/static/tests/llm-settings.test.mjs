import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("OpenAI defaults to GPT-5.6 Sol", async () => {
  const source = await readFile(new URL("../src/hooks/useLLMSettings.ts", import.meta.url), "utf8");
  const openAIPreset = source.match(/id: "openai",[\s\S]*?\n  \},/u)?.[0] ?? "";
  const defaultSettings = source.match(/const DEFAULT_SETTINGS[\s\S]*?\n\};/u)?.[0] ?? "";

  assert.match(openAIPreset, /model: "gpt-5\.6-sol"/u);
  assert.match(defaultSettings, /model: "gpt-5\.6-sol"/u);
});
