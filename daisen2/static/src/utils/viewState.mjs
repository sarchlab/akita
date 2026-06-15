// Single source of truth for encoding/decoding Daisen "view state" to and from
// URLs. Pure functions, no React — directly testable with `node --test` and
// imported by the TSX pages. See daisen2/docs/daisenbot-agent-plan.md
// (Phase 1, Workstreams A/B) for the schema and rationale.
//
// Schema (all single-value; no multi-select exists in the app):
//   /dashboard : starttime? endtime? filter? page? primary? secondary? widget?
//   /component : name  taskid?  starttime?  endtime?
//   /task      : id?  where?  kind?  sel?
//
// Existing param names (name, taskid, starttime, endtime, id, where) are kept
// for link back-compat; only new params (kind, sel, dashboard params) are added.

/** Canonical route paths. "/" is an alias of "/dashboard" on parse. */
export const ROUTES = {
  dashboard: "/dashboard",
  component: "/component",
  task: "/task",
};

/** Dashboard axis defaults — MUST match DashboardPage. Omitted from the URL. */
export const DASHBOARD_DEFAULTS = {
  primary: "ReqInCount",
  secondary: "AvgLatency",
};

/**
 * @typedef {Object} DashboardView
 * @property {"dashboard"} route
 * @property {number} [startTime]
 * @property {number} [endTime]
 * @property {string} [filter]
 * @property {number} [page]       0-indexed grid page
 * @property {string} [primary]    primary Y-axis metric
 * @property {string} [secondary]  secondary Y-axis metric
 * @property {string} [widget]     when set: render ONLY this component's chart
 */

/**
 * @typedef {Object} ComponentView
 * @property {"component"} route
 * @property {string} name
 * @property {string} [taskId]
 * @property {number} [startTime]
 * @property {number} [endTime]
 */

/**
 * @typedef {Object} TaskView
 * @property {"task"} route
 * @property {string} [id]
 * @property {string} [where]
 * @property {string} [kind]
 * @property {string} [sel]
 */

/** @typedef {DashboardView | ComponentView | TaskView} ViewState */

const isFiniteNum = (v) => typeof v === "number" && Number.isFinite(v);

/** Lossless numeric round-trip via String()/Number(). */
const numToParam = (n) => String(n);

/** @returns {number|undefined} a finite number, or undefined. */
const paramToNum = (s) => {
  if (s == null || s === "") return undefined;
  const n = Number(s);
  return Number.isFinite(n) ? n : undefined;
};

const setStr = (params, key, value) => {
  if (typeof value === "string" && value !== "") params.set(key, value);
};
const setNum = (params, key, value) => {
  if (isFiniteNum(value)) params.set(key, numToParam(value));
};

/**
 * Encode a view to a canonical "/path?query" string. Empty / default fields are
 * omitted, and keys are emitted in a fixed per-route order, so two equal views
 * always produce the identical URL (needed for link sharing and render dedup).
 * @param {ViewState} view
 * @returns {string}
 */
export function encodeView(view) {
  const params = new URLSearchParams();
  let path;

  switch (view && view.route) {
    case "component":
      path = ROUTES.component;
      setStr(params, "name", view.name);
      setStr(params, "taskid", view.taskId);
      setNum(params, "starttime", view.startTime);
      setNum(params, "endtime", view.endTime);
      break;

    case "task":
      path = ROUTES.task;
      setStr(params, "id", view.id);
      setStr(params, "where", view.where);
      setStr(params, "kind", view.kind);
      setStr(params, "sel", view.sel);
      break;

    case "dashboard":
    default:
      path = ROUTES.dashboard;
      setNum(params, "starttime", view.startTime);
      setNum(params, "endtime", view.endTime);
      setStr(params, "filter", view.filter);
      if (isFiniteNum(view.page) && view.page > 0) {
        params.set("page", String(Math.trunc(view.page)));
      }
      if (typeof view.primary === "string" && view.primary !== "" &&
          view.primary !== DASHBOARD_DEFAULTS.primary) {
        params.set("primary", view.primary);
      }
      if (typeof view.secondary === "string" && view.secondary !== "" &&
          view.secondary !== DASHBOARD_DEFAULTS.secondary) {
        params.set("secondary", view.secondary);
      }
      setStr(params, "widget", view.widget);
      break;
  }

  const search = params.toString();
  return search ? `${path}?${search}` : path;
}

/**
 * Map a pathname to a route. "/" and "/dashboard" (and anything unknown) →
 * dashboard; trailing slashes ignored.
 * @param {string} pathname
 * @returns {"dashboard"|"component"|"task"}
 */
export function routeForPath(pathname) {
  const p = (pathname || "/").replace(/\/+$/, "") || "/";
  if (p === ROUTES.component) return "component";
  if (p === ROUTES.task) return "task";
  return "dashboard";
}

/**
 * Parse a view from a pathname + query. `query` may be a string ("a=1&b=2", with
 * or without leading "?") or a URLSearchParams.
 * @param {string} pathname
 * @param {string|URLSearchParams} [query]
 * @returns {ViewState}
 */
export function parseView(pathname, query) {
  const params = query instanceof URLSearchParams
    ? query
    : new URLSearchParams((query ?? "").replace(/^\?/, ""));

  switch (routeForPath(pathname)) {
    case "component": {
      /** @type {ComponentView} */
      const v = { route: "component", name: params.get("name") ?? "" };
      const taskId = params.get("taskid");
      if (taskId) v.taskId = taskId;
      const s = paramToNum(params.get("starttime"));
      if (s !== undefined) v.startTime = s;
      const e = paramToNum(params.get("endtime"));
      if (e !== undefined) v.endTime = e;
      return v;
    }

    case "task": {
      /** @type {TaskView} */
      const v = { route: "task" };
      const id = params.get("id");
      if (id) v.id = id;
      const where = params.get("where");
      if (where) v.where = where;
      const kind = params.get("kind");
      if (kind) v.kind = kind;
      const sel = params.get("sel");
      if (sel) v.sel = sel;
      return v;
    }

    default: {
      /** @type {DashboardView} */
      const v = { route: "dashboard" };
      const s = paramToNum(params.get("starttime"));
      if (s !== undefined) v.startTime = s;
      const e = paramToNum(params.get("endtime"));
      if (e !== undefined) v.endTime = e;
      const filter = params.get("filter");
      if (filter) v.filter = filter;
      const page = paramToNum(params.get("page"));
      if (page !== undefined && page > 0) v.page = Math.trunc(page);
      const primary = params.get("primary");
      if (primary) v.primary = primary;
      const secondary = params.get("secondary");
      if (secondary) v.secondary = secondary;
      const widget = params.get("widget");
      if (widget) v.widget = widget;
      return v;
    }
  }
}

/**
 * Whether a view is the dashboard in single-widget mode (render one chart only).
 * @param {ViewState} view
 * @returns {boolean}
 */
export function isSingleWidget(view) {
  return !!view && view.route === "dashboard" &&
    typeof view.widget === "string" && view.widget !== "";
}
