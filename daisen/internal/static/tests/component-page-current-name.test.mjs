import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// The component page is pinned to the `name` from the URL: selecting a task only
// highlights it in place — it never re-scopes the view to the task's own component
// (the former issue-#156 scope-following behavior, and its `isWithinScope` helper,
// were removed). Re-centering on a task is an explicit "make current" action.
test("component page stays pinned to its URL name on selection", async () => {
  const source = await readFile(new URL("../src/pages/ComponentPage.tsx", import.meta.url), "utf8");

  // The active component is pinned to the URL `name`, with no scope-walk.
  assert.match(source, /const componentName = name;/);

  // The scope-containment helper and the old derived-location expression are gone.
  assert.doesNotMatch(source, /isWithinScope/);
  assert.doesNotMatch(source, /const componentName = selectedLocation/);

  // The side-panel breadcrumb and the location-scoped data sources all key off the
  // pinned componentName.
  assert.match(source, /breadcrumbSegments\(componentName\)/);
  assert.match(source, /useStackedCompInfo\(componentName,/);
  assert.match(source, /useComponentTimeline\(componentName,/);
  assert.match(source, /scope: componentName,/);
});
