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
 * A view across all routes. A single permissive shape (rather than a discriminated
 * union) so the TSX pages — which each already know their route — can read any
 * field and build patches without union-narrowing ceremony. The functions below
 * only ever read the fields relevant to `route`.
 *
 * @typedef {Object} ViewState
 * @property {"dashboard"|"component"|"task"} route
 * @property {string} [name]       component: component name
 * @property {string} [taskId]     component: selected task id
 * @property {string} [id]         task: active task id
 * @property {string} [where]      task: component filter
 * @property {string} [kind]       task: kind filter
 * @property {string} [sel]        task: selected task (browse mode)
 * @property {string} [filter]     dashboard: component-name filter
 * @property {number} [page]       dashboard: 0-indexed grid page
 * @property {string} [primary]    dashboard: primary Y-axis metric
 * @property {string} [secondary]  dashboard: secondary Y-axis metric
 * @property {string} [widget]     dashboard: when set, render ONLY this component's chart
 * @property {number} [startTime]  dashboard/component: view range start
 * @property {number} [endTime]    dashboard/component: view range end
 */

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
/**
 * Build the canonical path + URLSearchParams for a view (shared by encodeView
 * and encodeSearchParams). Empty / default fields are omitted; keys are emitted
 * in a fixed per-route order.
 * @param {ViewState} view
 * @returns {{ path: string, params: URLSearchParams }}
 */
function buildView(view) {
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
    default: {
      path = ROUTES.dashboard;
      const singleWidget = typeof view.widget === "string" && view.widget !== "";
      setNum(params, "starttime", view.startTime);
      setNum(params, "endtime", view.endTime);
      // filter + page only apply to the grid; omit them in single-widget mode so
      // equal single-widget views encode identically.
      if (!singleWidget) {
        setStr(params, "filter", view.filter);
        if (isFiniteNum(view.page) && view.page > 0) {
          params.set("page", String(Math.trunc(view.page)));
        }
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
  }

  return { path, params };
}

/**
 * Encode a view to a canonical "/path?query" string. Empty / default fields are
 * omitted, and keys are emitted in a fixed per-route order, so two equal views
 * always produce the identical URL (needed for link sharing and render dedup).
 * @param {ViewState} view
 * @returns {string}
 */
export function encodeView(view) {
  const { path, params } = buildView(view);
  const search = params.toString();
  return search ? `${path}?${search}` : path;
}

/**
 * Encode a view to a URLSearchParams (the query part only), for react-router's
 * setSearchParams. Same canonical rules as encodeView.
 * @param {ViewState} view
 * @returns {URLSearchParams}
 */
export function encodeSearchParams(view) {
  return buildView(view).params;
}

/**
 * Parse the view from `query`, apply a shallow `patch` (keys override; a key set
 * to undefined is dropped), and re-encode canonically. Convenience for
 * react-router's functional setSearchParams:
 *   setSearchParams((prev) => mergeParams("/dashboard", prev, { filter, page: 0 }), { replace: true })
 * @param {string} pathname
 * @param {string|URLSearchParams} query
 * @param {Object} patch
 * @returns {URLSearchParams}
 */
export function mergeParams(pathname, query, patch) {
  const base = parseView(pathname, query);
  return encodeSearchParams({ ...base, ...patch });
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

/** The concrete Daisen view paths (no "/" alias) — matches captureView's allow-list. */
const VIEW_PATHS = new Set([ROUTES.dashboard, ROUTES.component, ROUTES.task]);

/**
 * Whether `raw` is a root-relative, same-origin Daisen view path
 * (/dashboard, /component, or /task, with an optional query/fragment). Pure (no
 * `window`) so it is usable in markdown rendering and node tests. Rejects
 * absolute/protocol-relative URLs and any scheme (javascript:, data:, http:),
 * since those do not start with a single "/". This is the *tagging* gate; the
 * authoritative security check for rendering/navigation is captureView's
 * toSafeDaisenUrl, applied again at capture/click time.
 * @param {string} raw
 * @returns {boolean}
 */
export function isDaisenViewPath(raw) {
  if (typeof raw !== "string" || raw === "") return false;
  if (!raw.startsWith("/") || raw.startsWith("//")) return false;
  const pathname = raw.split(/[?#]/)[0];
  if (/[\s\\]/.test(pathname)) return false;
  return VIEW_PATHS.has(pathname);
}

/**
 * Canonicalize a Daisen view path to "/path?query" via parseView→encodeView, so two
 * URLs describing the same view compare equal (param order, defaults, and unknown
 * params are all normalized away). Returns null when `raw` is not a Daisen view path.
 * Used to key captured-view images by URL and match a cited URL to a render.
 * @param {string} raw
 * @returns {string|null}
 */
export function canonicalViewUrl(raw) {
  if (!isDaisenViewPath(raw)) return null;
  const [pathname, rest = ""] = raw.split("?");
  const query = rest.split("#")[0];
  return encodeView(parseView(pathname, query));
}
