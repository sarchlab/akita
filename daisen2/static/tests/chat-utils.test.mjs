import assert from "node:assert/strict";
import test from "node:test";

import { renderChatMarkdown } from "../src/utils/chatMarkdown.mjs";
import {
  FILE_UPLOAD_ACCEPT,
  IMAGE_UPLOAD_ACCEPT,
  isImageUploadCandidate,
  validateUploadedFile,
} from "../src/utils/uploadValidation.mjs";

test("renderChatMarkdown renders headings, bold, inline code, and rules", () => {
  const html = renderChatMarkdown("# Title\n\n**bold** and `code`\n\n---");

  assert.match(html, /<h1>Title<\/h1>/);
  assert.match(html, /<strong>bold<\/strong>/);
  assert.match(html, /<code>code<\/code>/);
  assert.match(html, /<hr>/);
});

test("renderChatMarkdown renders bullet lists", () => {
  const html = renderChatMarkdown("- one\n- two");

  assert.match(html, /<ul>/);
  assert.match(html, /<li>one<\/li>/);
  assert.match(html, /<li>two<\/li>/);
});

test("renderChatMarkdown renders a markdown pipe table", () => {
  const html = renderChatMarkdown(
    "| Component | Reqs |\n| --- | --- |\n| L1Cache | 128 |\n| L2Cache | 64 |",
  );

  assert.match(html, /<table>/);
  assert.match(html, /<thead>/);
  assert.match(html, /<th>Component<\/th>/);
  assert.match(html, /<th>Reqs<\/th>/);
  assert.match(html, /<td>L1Cache<\/td>/);
  assert.match(html, /<td>128<\/td>/);
  assert.match(html, /<td>L2Cache<\/td>/);
  assert.match(html, /<td>64<\/td>/);
});

test("renderChatMarkdown honors table column alignment and inline formatting", () => {
  const html = renderChatMarkdown("| Name | Value |\n|:---|---:|\n| **a** | `1` |");

  assert.match(html, /<th style="text-align:left">Name<\/th>/);
  assert.match(html, /<th style="text-align:right">Value<\/th>/);
  assert.match(html, /<td style="text-align:left"><strong>a<\/strong><\/td>/);
  assert.match(html, /<td style="text-align:right"><code>1<\/code><\/td>/);
});

test("renderChatMarkdown leaves non-table pipe text alone", () => {
  const html = renderChatMarkdown("a | b is not a table");

  assert.doesNotMatch(html, /<table/);
});

test("renderChatMarkdown escapes raw HTML from the (untrusted) model", () => {
  const html = renderChatMarkdown("<img src=x onerror=alert(1)>");

  assert.doesNotMatch(html, /<img/);
  assert.match(html, /&lt;img/);
});

test("renderChatMarkdown opens links in a new tab with a safe rel", () => {
  const html = renderChatMarkdown("See [docs](https://example.com).");

  assert.match(html, /<a [^>]*href="https:\/\/example\.com"[^>]*>docs<\/a>/);
  assert.match(html, /target="_blank"/);
  assert.match(html, /rel="noopener noreferrer"/);
});

test("renderChatMarkdown renders a Daisen view image as an evidence figure", () => {
  const html = renderChatMarkdown(
    "See it: ![L2 occupancy](/component?name=L2Cache&starttime=0&endtime=379102000)",
  );

  // A thumbnail carrying the canonical view url and NO src (filled in by React),
  // plus a caption that links to the same view in a new tab.
  assert.match(
    html,
    /<img class="daisen-evidence" data-view-url="\/component\?name=L2Cache&amp;starttime=0&amp;endtime=379102000" alt="L2 occupancy">/,
  );
  assert.match(
    html,
    /<a class="daisen-evidence-link" data-view-url="[^"]+" href="\/component\?name=L2Cache[^"]*" target="_blank" rel="noopener noreferrer">L2 occupancy ↗<\/a>/,
  );
  assert.doesNotMatch(html, /class="daisen-evidence"[^>]*\ssrc=/); // no src on the thumbnail
});

test("renderChatMarkdown canonicalizes the cited view url (param order, unknowns)", () => {
  const html = renderChatMarkdown("![x](/component?endtime=20&name=C&starttime=10&bogus=1)");

  assert.match(html, /data-view-url="\/component\?name=C&amp;starttime=10&amp;endtime=20"/);
  assert.doesNotMatch(html, /bogus/);
});

test("renderChatMarkdown leaves non-Daisen and unsafe images as plain markdown", () => {
  const external = renderChatMarkdown("![pic](https://example.com/p.png)");
  assert.doesNotMatch(external, /daisen-evidence/);
  assert.match(external, /<img src="https:\/\/example\.com\/p\.png" alt="pic">/);

  // root-relative but not a view path, and protocol-relative — neither is tagged.
  assert.doesNotMatch(renderChatMarkdown("![x](/etc/passwd)"), /daisen-evidence/);
  assert.doesNotMatch(renderChatMarkdown("![x](//evil.com/component)"), /daisen-evidence/);
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
