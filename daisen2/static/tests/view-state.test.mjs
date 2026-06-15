import assert from "node:assert/strict";
import test from "node:test";

import {
  ROUTES,
  DASHBOARD_DEFAULTS,
  encodeView,
  encodeSearchParams,
  parseView,
  routeForPath,
  isSingleWidget,
} from "../src/utils/viewState.mjs";

// Round-trip: parse(encode(v)) === v for fully-specified, non-default views.
const roundTripCases = [
  {
    route: "component",
    name: "GPU[0].L2Cache",
    taskId: "abc123",
    startTime: 300.000000123,
    endTime: 500.5,
  },
  { route: "component", name: "Simple" },
  {
    route: "task",
    id: "d240btg3fvio1hp2d3eg",
    where: "GPU[1].CU[3]",
    kind: "read",
    sel: "child42",
  },
  { route: "task" },
  {
    route: "dashboard",
    startTime: 10,
    endTime: 20,
    filter: "L2",
    page: 2,
    primary: "BufferPressure",
    secondary: "PendingReqOut",
    widget: "GPU[0].DRAM",
  },
  { route: "dashboard" },
];

for (const view of roundTripCases) {
  test(`round-trip: ${encodeView(view)}`, () => {
    const url = encodeView(view);
    const [pathname, search = ""] = url.split("?");
    assert.deepEqual(parseView(pathname, search), view);
  });
}

test("encodeView uses canonical fixed key order regardless of object key order", () => {
  const a = { route: "dashboard", widget: "X", filter: "f", startTime: 1, endTime: 2 };
  const b = { route: "dashboard", endTime: 2, startTime: 1, filter: "f", widget: "X" };
  assert.equal(encodeView(a), encodeView(b));
  assert.equal(encodeView(a), "/dashboard?starttime=1&endtime=2&filter=f&widget=X");
});

test("dashboard defaults are omitted from the URL and recovered as undefined", () => {
  const url = encodeView({
    route: "dashboard",
    primary: DASHBOARD_DEFAULTS.primary,
    secondary: DASHBOARD_DEFAULTS.secondary,
    page: 0,
  });
  assert.equal(url, "/dashboard");
  assert.deepEqual(parseView("/dashboard"), { route: "dashboard" });
});

test("non-default axis metrics are encoded; defaults are not", () => {
  const url = encodeView({
    route: "dashboard",
    primary: "AvgLatency",
    secondary: DASHBOARD_DEFAULTS.secondary,
  });
  assert.equal(url, "/dashboard?primary=AvgLatency");
});

test('"/" and "/dashboard" both parse to the dashboard route', () => {
  assert.equal(routeForPath("/"), "dashboard");
  assert.equal(routeForPath("/dashboard"), "dashboard");
  assert.equal(routeForPath("/dashboard/"), "dashboard");
  assert.deepEqual(parseView("/", "filter=mem"), { route: "dashboard", filter: "mem" });
});

test("encodeView canonicalizes the dashboard route to /dashboard", () => {
  assert.equal(encodeView({ route: "dashboard" }), "/dashboard");
});

test("component name with special characters round-trips through URL encoding", () => {
  const view = { route: "component", name: "GPU[0].L2$Cache (main)" };
  const url = encodeView(view);
  const [pathname, search = ""] = url.split("?");
  assert.deepEqual(parseView(pathname, search), view);
});

test("high-precision time round-trips exactly", () => {
  const view = { route: "component", name: "C", startTime: 0.000000000123456, endTime: 1234.56789 };
  const [pathname, search = ""] = encodeView(view).split("?");
  const parsed = parseView(pathname, search);
  assert.equal(parsed.startTime, view.startTime);
  assert.equal(parsed.endTime, view.endTime);
});

test("page <= 0 is omitted; page > 0 is kept (0-indexed)", () => {
  assert.equal(encodeView({ route: "dashboard", page: 0 }), "/dashboard");
  assert.equal(encodeView({ route: "dashboard", page: -1 }), "/dashboard");
  assert.equal(encodeView({ route: "dashboard", page: 3 }), "/dashboard?page=3");
  assert.equal(parseView("/dashboard", "page=0").page, undefined);
  assert.equal(parseView("/dashboard", "page=3").page, 3);
});

test("empty component name parses to name='' and encodes to bare /component", () => {
  assert.deepEqual(parseView("/component", ""), { route: "component", name: "" });
  assert.equal(encodeView({ route: "component", name: "" }), "/component");
});

test("isSingleWidget reflects the widget param", () => {
  assert.equal(isSingleWidget(parseView("/dashboard", "widget=GPU[0].DRAM")), true);
  assert.equal(isSingleWidget(parseView("/dashboard", "")), false);
  assert.equal(isSingleWidget(parseView("/component", "name=X")), false);
});

test("leading '?' in query is tolerated", () => {
  assert.deepEqual(parseView("/task", "?id=t1"), { route: "task", id: "t1" });
});

test("encodeSearchParams matches the query part of encodeView", () => {
  for (const view of roundTripCases) {
    const url = encodeView(view);
    const expectedSearch = url.includes("?") ? url.split("?")[1] : "";
    assert.equal(encodeSearchParams(view).toString(), expectedSearch);
  }
});

test("encodeSearchParams returns a usable URLSearchParams for setSearchParams", () => {
  const params = encodeSearchParams({ route: "task", id: "t1", kind: "read" });
  assert.equal(params.get("id"), "t1");
  assert.equal(params.get("kind"), "read");
  assert.equal(params.get("where"), null);
});

test("ROUTES are the expected canonical paths", () => {
  assert.equal(ROUTES.dashboard, "/dashboard");
  assert.equal(ROUTES.component, "/component");
  assert.equal(ROUTES.task, "/task");
});
