import assert from "node:assert/strict";
import test from "node:test";

import { parseMarkdown } from "../.tmp-tests/utils/chatMarkdown.js";
import {
  isImageUploadCandidate,
  validateUploadedFile,
} from "../.tmp-tests/utils/uploadValidation.js";

test("parseMarkdown supports legacy inline \\( ... \\) delimiters", () => {
  const html = parseMarkdown("Inline math: \\(x^2 + y^2\\).");

  assert.match(html, /katex/);
  assert.doesNotMatch(html, /\\\(/);
  assert.doesNotMatch(html, /\\\)/);
});

test("parseMarkdown supports legacy block \\[ ... \\] delimiters", () => {
  const html = parseMarkdown("Before\\n\\[x^2 + y^2 = z^2\\]\\nAfter");

  assert.match(html, /katex-display/);
});

test("parseMarkdown continues supporting dollar delimiters", () => {
  const html = parseMarkdown("$x+1$");

  assert.match(html, /katex/);
});

test("validateUploadedFile enforces legacy file extension and size limits", () => {
  assert.equal(validateUploadedFile({ name: "trace.csv", size: 8 * 1024 }, "file").valid, true);

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
