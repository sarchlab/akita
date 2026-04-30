import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const readSource = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("chat panel production path imports shared markdown and upload helpers", async () => {
  const source = await readSource("../src/chatpanel.ts");

  assert.match(source, /from "\.\/utils\/chatMarkdown\.mjs"/);
  assert.match(source, /renderChatMarkdown/);
  assert.match(source, /renderMathInElement/);
  assert.match(source, /from "\.\/utils\/uploadValidation\.mjs"/);
  assert.match(source, /validateUploadedFile/);
  assert.match(source, /isImageUploadCandidate/);
  assert.doesNotMatch(source, /function convertMarkdownToHTML/);
  assert.doesNotMatch(source, /function autoWrapMath/);
});

test("app production path imports shared mode response parser", async () => {
  const source = await readSource("../src/app.ts");

  assert.match(source, /from "\.\/utils\/mode\.mjs"/);
  assert.match(source, /parseModeResponse\(await response\.text\(\)\)/);
});
