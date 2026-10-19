import assert from "node:assert/strict";
import test from "node:test";
import { computeSiblingGroups } from "../src/utils/siblingGroups.mjs";

test("only siblings with a shared external parent form a dependency group", () => {
  const tasks = [
    { id: 1, parent_id: 0 },
    { id: 2, parent_id: 0 },
    { id: 3, parent_id: 99 },
    { id: 4, parent_id: 99 },
    { id: 5, parent_id: 1 },
    { id: 6, parent_id: 1 },
    { id: 7, parent_id: 100 },
  ];
  // Roots are unrelated; internal children already nest, and lone children
  // have no sibling dependency. Only 3 and 4 share an external request.
  assert.deepEqual(computeSiblingGroups(tasks), new Map([["3", "99"], ["4", "99"]]));
});
