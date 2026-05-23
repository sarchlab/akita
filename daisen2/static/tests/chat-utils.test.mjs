import assert from "node:assert/strict";
import test from "node:test";

import {
  parseMarkdown,
  renderChatMarkdown,
  renderMathInElement,
} from "../src/utils/chatMarkdown.mjs";
import {
  FILE_UPLOAD_ACCEPT,
  IMAGE_UPLOAD_ACCEPT,
  isImageUploadCandidate,
  validateUploadedFile,
} from "../src/utils/uploadValidation.mjs";

test("parseMarkdown supports legacy inline \\( ... \\) delimiters", () => {
  const html = parseMarkdown("Inline math: \\(x^2 + y^2\\).");

  assert.match(html, /katex/);
  assert.doesNotMatch(html, /\\\(/);
  assert.doesNotMatch(html, /\\\)/);
});

test("parseMarkdown supports legacy block \\[ ... \\] delimiters", () => {
  const html = parseMarkdown("Before\n\\[x^2 + y^2 = z^2\\]\nAfter");

  assert.match(html, /katex-display/);
});

test("parseMarkdown continues supporting dollar delimiters", () => {
  const html = parseMarkdown("$x+1$");

  assert.match(html, /katex/);
});

test("renderChatMarkdown preserves production markdown and math placeholders", () => {
  const html = renderChatMarkdown("# Title\n**bold** and \\(x+1\\)\n---");

  assert.match(html, /<h3>Title<\/h3>/);
  assert.match(html, /<b>bold<\/b>/);
  assert.match(html, /<span class="math" data-display="inline">x\+1<\/span>/);
  assert.match(html, /<hr>/);
});

test("renderChatMarkdown preserves trusted html code fences used by chat responses", () => {
  const html = renderChatMarkdown("```html\n<table><tr><td>42</td></tr></table>\n```");

  assert.equal(html, "<table><tr><td>42</td></tr></table>");
});

test("renderMathInElement renders production math placeholders with KaTeX", () => {
  const inlineMath = {
    textContent: "x+1",
    innerHTML: "",
    getAttribute: (name) => (name === "data-display" ? "inline" : null),
  };
  const root = { querySelectorAll: () => [inlineMath] };

  renderMathInElement(root);

  assert.match(inlineMath.innerHTML, /katex/);
  assert.doesNotMatch(inlineMath.innerHTML, /x\+1<\/span>$/);
});

test("validateUploadedFile enforces legacy file extension and size limits", () => {
  assert.equal(validateUploadedFile({ name: "trace.csv", size: 8 * 1024 }, "file").valid, true);
  assert.equal(FILE_UPLOAD_ACCEPT, ".sqlite,.sqlite3,.csv,.txt,.json,.py,.js,.c,.cpp,.java");

  const invalidType = validateUploadedFile({ name: "trace.md", size: 4 * 1024 }, "file");
  assert.equal(invalidType.valid, false);
  assert.equal(
    invalidType.error,
    "Invalid file type. Allowed: .sqlite, .sqlite3, .csv, .txt, .json, .py, .js, .c, .cpp, .java",
  );

  const tooLarge = validateUploadedFile({ name: "trace.csv", size: 40 * 1024 }, "file");
  assert.equal(tooLarge.valid, false);
  assert.equal(tooLarge.error, "File too large. Max size is 32 KB.");
});

test("validateUploadedFile enforces image constraints while preserving screenshots", () => {
  assert.equal(validateUploadedFile({ name: "plot.PNG", size: 120 * 1024 }, "image").valid, true);
  assert.equal(IMAGE_UPLOAD_ACCEPT, ".png,.jpg,.jpeg");

  const tooLargeImage = validateUploadedFile({ name: "plot.png", size: 300 * 1024 }, "image");
  assert.equal(tooLargeImage.valid, false);
  assert.equal(tooLargeImage.error, "File too large. Max size is 256 KB.");

  const screenshot = validateUploadedFile(
    { name: "screenshot-1.png", size: 2 * 1024 * 1024 },
    "image-screenshot",
  );
  assert.equal(screenshot.valid, true);
});

test("isImageUploadCandidate detects dropped images by MIME type or extension", () => {
  assert.equal(isImageUploadCandidate({ name: "figure.jpeg", type: "" }), true);
  assert.equal(isImageUploadCandidate({ name: "figure.bin", type: "image/png" }), true);
  assert.equal(isImageUploadCandidate({ name: "notes.txt", type: "text/plain" }), false);
});
