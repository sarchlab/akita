import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Issue #156 + scope behavior: the view follows a selected task to a *different*
// component, but stays in the current scope when the task already lives within it
// (clicking a task in ROB.Top.incoming while viewing ROB keeps you at ROB).
test("component page follows out-of-scope tasks but stays in scope otherwise", async () => {
  const source = await readFile(new URL("../src/pages/ComponentPage.tsx", import.meta.url), "utf8");

  // A scope-containment helper drives the decision.
  assert.match(source, /function isWithinScope\(location: string, scope: string\)/);

  // The location in view follows the task only when it falls outside the scope.
  assert.match(source, /const componentName = selectedLocation && !isWithinScope\(selectedLocation, name\) \? selectedLocation : name;/);
  assert.doesNotMatch(source, /const componentName = selectedTask\?\.location \|\| name;/);

  // The side-panel header breadcrumb is built from the derived location.
  assert.match(source, /breadcrumbSegments\(componentName\)/);

  // The location-scoped data sources are all scoped to the derived location's subtree.
  assert.match(source, /useStackedCompInfo\(componentName,/);
  assert.match(source, /useComponentTimeline\(componentName,/);
  assert.match(source, /scope: componentName,/);

  // Selecting a task keeps the active scope (componentName, not the stale URL
  // `name`) unless the task is outside it.
  assert.match(source, /params\.set\("name", task\.location && !isWithinScope\(task\.location, componentName\) \? task\.location : componentName\)/);
});
