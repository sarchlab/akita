import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const readSource = (path) => readFile(new URL(path, import.meta.url), "utf8");

test("chat panel production path renders assistant markdown and math through shared helpers", async () => {
  const source = await readSource("../src/chatpanel.ts");

  assert.match(source, /from "\.\/utils\/chatMarkdown\.mjs"/);
  assert.match(
    source,
    /botDiv\.innerHTML = "<b>Daisen Bot:<\/b> " \+ renderChatMarkdown\(firstContent\.text\);/,
  );
  assert.match(source, /renderChatMarkdown\(gptResponseContent\)/);
  assert.match(source, /renderMathInElement\(messagesDiv\);/);
  assert.match(source, /renderMathInElement\(botDiv\);/);
  assert.doesNotMatch(source, /function convertMarkdownToHTML/);
  assert.doesNotMatch(source, /function autoWrapMath/);
});

test("chat panel production path validates uploads and dropped-image detection through shared helpers", async () => {
  const source = await readSource("../src/chatpanel.ts");

  assert.match(source, /from "\.\/utils\/uploadValidation\.mjs"/);
  assert.match(source, /if \(isImageUploadCandidate\(file\)\) {\n\s+handleImageUpload\(file\);/);
  assert.match(source, /const validation = validateUploadedFile\(file, "file"\);/);
  assert.match(source, /const validation = validateUploadedFile\(file, "image"\);/);
  assert.match(source, /fileInput\.accept = FILE_UPLOAD_ACCEPT;/);
  assert.match(source, /imageInput\.accept = IMAGE_UPLOAD_ACCEPT;/);
  assert.doesNotMatch(source, /const allowed = \[/);
});

test("app production path imports shared mode response parser", async () => {
  const source = await readSource("../src/app.ts");

  assert.match(source, /from "\.\/utils\/mode\.mjs"/);
  assert.match(source, /parseModeResponse\(await response\.text\(\)\)/);
});
