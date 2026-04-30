import assert from "node:assert/strict";
import { describe, it } from "node:test";

import { buildTaskColorMap } from "../src/taskColorMap.mjs";

function recordingChroma(calls, colors) {
  return {
    cubehelix() {
      calls.push(["cubehelix"]);
      return {
        gamma(value) {
          calls.push(["gamma", value]);
          return this;
        },
        lightness(value) {
          calls.push(["lightness", value]);
          return this;
        },
        scale() {
          calls.push(["scale"]);
          return {
            colors(count) {
              calls.push(["colors", count]);
              return colors;
            },
          };
        },
      };
    },
  };
}

describe("task color map", () => {
  it("preserves the cubehelix scale parameters and color offset", () => {
    const calls = [];
    const chroma = recordingChroma(calls, ["unused", "#111111", "#222222"]);

    const colorMap = buildTaskColorMap(
      [
        { kind: "kernel", what: "zeta" },
        { kind: "kernel", what: "alpha" },
        { kind: "kernel", what: "alpha" },
      ],
      chroma,
    );

    assert.deepEqual(calls, [
      ["cubehelix"],
      ["gamma", 0.7],
      ["lightness", [0.1, 0.7]],
      ["scale"],
      ["colors", 3],
    ]);
    assert.deepEqual(colorMap, {
      "kernel-alpha": "#111111",
      "kernel-zeta": "#222222",
    });
  });
});
