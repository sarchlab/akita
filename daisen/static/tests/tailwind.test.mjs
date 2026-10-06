import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import test from "node:test";
import postcss from "postcss";
import { twMerge } from "tailwind-merge";
import config from "../postcss.config.js";

test("the production CSS pipeline preserves the theme and focus utilities", async () => {
  const from = fileURLToPath(new URL("../src/styles.css", import.meta.url));
  const plugins = await Promise.all(
    Object.entries(config.plugins).map(async ([name, options]) => {
      const { default: plugin } = await import(name);
      return plugin(options);
    }),
  );
  const result = await postcss(plugins).process(await readFile(from, "utf8"), { from });
  const declarations = (selector) => {
    const values = [];
    result.root.walkRules(selector, (rule) => {
      rule.walkDecls((decl) => values.push(`${decl.prop}: ${decl.value}`));
    });
    return values;
  };

  assert.ok(declarations(".bg-primary").includes("background-color: hsl(var(--primary))"));
  assert.ok(declarations(".rounded-md").includes("border-radius: calc(var(--radius) - 2px)"));
  assert.ok(declarations("*").includes("border-color: hsl(var(--border))"));
  assert.ok(declarations(".focus-visible\\:outline-hidden:focus-visible").includes("outline: 2px solid transparent"));
  assert.ok(declarations(".shadow-xs").some((value) => value.startsWith("box-shadow:")));
  result.root.walkAtRules((rule) => {
    assert.ok(!["tailwind", "apply", "config"].includes(rule.name), `uncompiled @${rule.name}`);
  });
});

test("class overrides recognize Tailwind 4 utility names", () => {
  assert.equal(twMerge("shadow-xs", "shadow-lg"), "shadow-lg");
  assert.equal(twMerge("outline-hidden", "outline-none"), "outline-none");
});
