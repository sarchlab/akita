import assert from "node:assert/strict";
import test from "node:test";

import { parseModeResponse } from "../.tmp-tests/hooks/useMode.js";

test("parseModeResponse parses JSON mode contract", () => {
  assert.equal(parseModeResponse('{"mode":"live"}'), "live");
  assert.equal(parseModeResponse('{"mode":"replay"}'), "replay");
});

test("parseModeResponse supports plain-text fallback responses", () => {
  assert.equal(parseModeResponse("live"), "live");
  assert.equal(parseModeResponse(" replay\n"), "replay");
});

test("parseModeResponse rejects unsupported mode payloads", () => {
  assert.equal(parseModeResponse('{"mode":"other"}'), null);
  assert.equal(parseModeResponse('{"unexpected":"live"}'), null);
  assert.equal(parseModeResponse(""), null);
});
