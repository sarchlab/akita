// Two tasks in this component that share a parent living OUTSIDE the component
// are the same request's phases (e.g. a port's incoming_buffer + req_in under
// one requester). buildTaskTree cannot nest them here (the parent is in another
// component), so group them by that parent id for packing and a dependency
// arrow. Parents that are in-component already nest their children, so they are
// skipped; only a parent with >= 2 in-component children forms a group, so a
// lone child never pays for an extra grouping row.
/** @param {Array<{id: number, parent_id: number}>} tasks */
export function computeSiblingGroups(tasks) {
  const inComponent = new Set(tasks.map((task) => String(task.id)));
  /** @type {Map<string, string[]>} */
  const byParent = new Map();
  for (const task of tasks) {
    if (!task.parent_id) continue;
    const pid = String(task.parent_id);
    if (inComponent.has(pid)) continue;
    let ids = byParent.get(pid);
    if (!ids) {
      ids = [];
      byParent.set(pid, ids);
    }
    ids.push(String(task.id));
  }

  /** @type {Map<string, string>} */
  const groupOf = new Map();
  for (const [pid, ids] of byParent) {
    if (ids.length < 2) continue;
    for (const id of ids) groupOf.set(id, pid);
  }
  return groupOf;
}
