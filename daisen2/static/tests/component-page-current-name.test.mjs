import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

// Regression for issue #156: the current component name (and the component view)
// must follow the selected task's location, so clicking a parent task or subtask
// navigates to that task's component instead of staying on the original one.
test("component page tracks the selected task's component", async () => {
  const source = await readFile(new URL("../src/pages/ComponentPage.tsx", import.meta.url), "utf8");

  // The component in view is derived from the selected task's location.
  assert.match(source, /const componentName = selectedTask\?\.location \|\| name;/);

  // The side-column label shows the derived component, not the raw URL name.
  assert.match(source, /daisen1-location-label">\{componentName\}/);
  assert.doesNotMatch(source, /daisen1-location-label">\{name\}/);

  // Component-scoped data (metric line and timeline query) uses the derived name.
  assert.match(source, /useCompInfo\(componentName,/);
  assert.match(source, /where: componentName,/);

  // Selecting a task records that task's component in the URL.
  assert.match(source, /params\.set\("name", task\.location \|\| name\)/);
});
