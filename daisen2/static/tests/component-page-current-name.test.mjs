import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Regression for issue #156: the current component name (and the component view)
// must follow the selected task's location, so clicking a parent task or subtask
// navigates to that task's component instead of staying on the original one.
test("component page tracks the selected task's component", async () => {
  const source = await readFile(new URL("../src/pages/ComponentPage.tsx", import.meta.url), "utf8");

  // The location in view is derived from the selected task's location.
  assert.match(source, /const componentName = selectedTask\?\.location \|\| name;/);

  // The side-panel header breadcrumb is built from the derived location, not the
  // raw URL name.
  assert.match(source, /breadcrumbSegments\(componentName\)/);

  // The location-scoped data sources (blocking-reason bars, occupancy timeline,
  // and raw-task query) are all scoped to the derived location's subtree.
  assert.match(source, /useStackedCompInfo\(componentName,/);
  assert.match(source, /useComponentTimeline\(componentName,/);
  assert.match(source, /scope: componentName,/);

  // Selecting a task records that task's component in the URL.
  assert.match(source, /params\.set\("name", task\.location \|\| name\)/);
});
