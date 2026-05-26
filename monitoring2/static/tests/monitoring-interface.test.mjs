import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("monitoring2 presents one monitor surface without product tabs", async () => {
  const app = await readFile(new URL("../src/App.tsx", import.meta.url), "utf8");
  const layout = await readFile(new URL("../src/components/Layout.tsx", import.meta.url), "utf8");
  const page = await readFile(new URL("../src/pages/LivePage.tsx", import.meta.url), "utf8");

  assert.match(app, /<Route index element={<Navigate to="\/execution" replace/);
  assert.match(app, /path="execution" element={<ProgressPage/);
  assert.match(app, /path="progress" element={<ProgressPage/);
  assert.match(app, /path="monitor" element={<LivePage/);
  assert.match(app, /path="analysis" element={<AnalysisPage/);
  assert.match(app, /path="debug" element={<DebugPage/);
  assert.match(app, /path="profiling" element={<ProfilingPage/);
  assert.match(app, /path="task" element={<LivePage/);
  assert.match(app, /path="component" element={<LivePage/);
  assert.match(app, /path="dashboard" element={<LivePage/);
  assert.match(layout, /Execution[\s\S]*Monitor[\s\S]*Analysis[\s\S]*Debug[\s\S]*Profiling/);
  assert.match(layout, /Monitor/);
  assert.match(layout, /Execution/);
  assert.match(layout, /Analysis/);
  assert.match(layout, /Debug/);
  assert.match(layout, /Profiling/);
  assert.match(layout, /role="tablist"/);
  assert.doesNotMatch(layout, /Dashboard|Task|Component/);
  assert.doesNotMatch(page, /Live Execution/);
});

test("monitoring2 page supports component selection and tracing controls", async () => {
  const page = await readFile(new URL("../src/pages/LivePage.tsx", import.meta.url), "utf8");
  const smartValue = await readFile(new URL("../src/utils/smartValue.ts", import.meta.url), "utf8");

  assert.match(page, /\/api\/list_components/);
  assert.match(page, /\/api\/field\//);
  assert.match(page, /MONITOR_SECTIONS/);
  assert.match(page, /Ports/);
  assert.match(page, /Spec/);
  assert.match(page, /State/);
  assert.doesNotMatch(page, /Ports \/ Spec \/ State/);
  assert.match(page, /TickingComponent\.PortOwnerBase\.ports/);
  assert.match(page, /border-b last:border-b-0/);
  assert.match(page, /min-h-0 overflow-auto bg-white/);
  assert.match(page, /expandedFields/);
  assert.match(page, /openSectionField/);
  assert.match(page, /withoutExpandedSubtree/);
  assert.match(page, /disabled=\{!canToggle\}/);
  assert.match(page, /ChevronRight/);
  assert.match(page, /ChevronDown/);
  assert.match(page, /ActionIcon/);
  assert.match(page, /aria-label=\{`\$\{actionLabel\}/);
  assert.doesNotMatch(page, /Open Field/);
  assert.doesNotMatch(page, /onDoubleClick/);
  assert.doesNotMatch(page, /grid h-full min-h-0 grid-rows-3 gap-3/);
  assert.doesNotMatch(page, /flex min-h-0 flex-col overflow-hidden rounded border bg-white/);
  assert.doesNotMatch(page, /\/api\/component\//);
  assert.match(page, /chooseComponent/);
  assert.match(page, /formatPicosecondsAsNanoseconds\(now\)/);
  assert.doesNotMatch(page, /smartString\(now\)/);
  assert.match(smartValue, /formatPicosecondsAsNanoseconds/);
  assert.match(smartValue, /value \/ 1000/);
  assert.match(smartValue, /ns/);
  assert.doesNotMatch(page, /Start Tracing/);
  assert.doesNotMatch(page, /Stop Tracing/);
  assert.doesNotMatch(page, /Pause Tracing/);
  assert.doesNotMatch(page, /\/api\/trace\//);
  assert.doesNotMatch(page, /Tick Selected/);
});

test("monitoring2 debug page supports manual component ticks", async () => {
  const page = await readFile(new URL("../src/pages/DebugPage.tsx", import.meta.url), "utf8");

  assert.match(page, /Debug/);
  assert.match(page, /\/api\/list_components/);
  assert.match(page, /Tick Selected/);
  assert.match(page, /Schedule Tick/);
  assert.match(page, /\/api\/tick\//);
});

test("monitoring2 page supports buffer analysis and profiling", async () => {
  const livePage = await readFile(new URL("../src/pages/LivePage.tsx", import.meta.url), "utf8");
  const analysisPage = await readFile(new URL("../src/pages/AnalysisPage.tsx", import.meta.url), "utf8");
  const progressPage = await readFile(new URL("../src/pages/ProgressPage.tsx", import.meta.url), "utf8");
  const profilingPage = await readFile(new URL("../src/pages/ProfilingPage.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(livePage, /Buffer Level Analysis/);
  assert.doesNotMatch(livePage, /\/api\/hangdetector\/buffers/);
  assert.doesNotMatch(livePage, /monitorTab ===/);
  assert.match(analysisPage, /Analysis/);
  assert.match(analysisPage, /Aggregate Buffer Level/);
  assert.match(analysisPage, /\/api\/hangdetector\/buffers/);
  assert.match(progressPage, /Execution/);
  assert.match(progressPage, /\/api\/progress/);
  assert.match(progressPage, /Tracing/);
  assert.match(progressPage, /Start Tracing/);
  assert.match(progressPage, /Stop Tracing/);
  assert.match(progressPage, /TraceActionIcon/);
  assert.match(progressPage, /isTracing \? Square : Play/);
  assert.match(progressPage, /post\(isTracing \? "\/api\/trace\/end" : "\/api\/trace\/start"\)/);
  assert.match(progressPage, /\/api\/trace\/start/);
  assert.match(progressPage, /\/api\/trace\/end/);
  assert.match(progressPage, /\/api\/trace\/is_tracing/);
  assert.match(progressPage, /\/api\/trace\/storage/);
  assert.match(progressPage, /TraceStorageState/);
  assert.match(progressPage, /SQLite file/);
  assert.match(progressPage, /Available disk/);
  assert.match(progressPage, /formatBytes/);
  assert.match(profilingPage, /Profiling/);
  assert.match(profilingPage, /Capture CPU Profile/);
  assert.doesNotMatch(profilingPage, /Latest CPU Profile/);
  assert.match(profilingPage, /Seconds/);
  assert.match(profilingPage, /CPU Call Graph/);
  assert.match(profilingPage, /activeProfileTab/);
  assert.match(profilingPage, /CPU profile views/);
  assert.match(profilingPage, /profileSummaryText/);
  assert.match(profilingPage, /CallGraph/);
  assert.match(profilingPage, /hotPathNodeIDs/);
  assert.match(profilingPage, /hotPathEdgeIDs/);
  assert.match(profilingPage, /h-\[32rem\]/);
  assert.match(profilingPage, /formatSampleCount/);
  assert.match(profilingPage, /profileValueInfo/);
  assert.match(profilingPage, /formatProfileValue/);
  assert.doesNotMatch(profilingPage, /samples \{formatSampleCount\(node\.value\)\}/);
  assert.match(profilingPage, /skippedFrames/);
  assert.match(profilingPage, /componentGap/);
  assert.match(profilingPage, /drawableEdges/);
  assert.match(profilingPage, /CALL_GRAPH_BUTTON_ZOOM_STEP/);
  assert.match(profilingPage, /CALL_GRAPH_WHEEL_ZOOM_RATE/);
  assert.match(profilingPage, /CALL_GRAPH_MAX_WHEEL_DELTA/);
  assert.match(profilingPage, /addEventListener\("wheel"/);
  assert.match(profilingPage, /passive: false/);
  assert.match(profilingPage, /overscroll-contain/);
  assert.doesNotMatch(profilingPage, /rounded border bg-slate-50 p-2/);
  assert.doesNotMatch(profilingPage, /h-\[32rem\] overscroll-contain overflow-hidden rounded border bg-white/);
  assert.match(profilingPage, /onPointerDown/);
  assert.match(profilingPage, /ZoomIn/);
  assert.match(profilingPage, /ZoomOut/);
  assert.match(profilingPage, /RotateCcw/);
  assert.match(profilingPage, /RESOURCE_SAMPLE_INTERVAL_MS = 1000/);
  assert.match(profilingPage, /MAX_SECOND_SAMPLES = 60/);
  assert.match(profilingPage, /MAX_MINUTE_SAMPLES = 60/);
  assert.match(profilingPage, /Last minute/);
  assert.match(profilingPage, /per-second samples/);
  assert.match(profilingPage, /Last 60 minutes/);
  assert.match(profilingPage, /per-minute averages/);
  assert.match(profilingPage, /mb-2 flex flex-wrap items-center justify-between gap-2/);
  assert.match(profilingPage, /title="Last 60 minutes"[\s\S]*title="Last minute"/);
  assert.match(profilingPage, /TrendSegmentChart/);
  assert.match(profilingPage, /onMouseEnter/);
  assert.match(profilingPage, /h-\[4\.5rem\] w-full/);
  assert.match(profilingPage, /const chartTop = 18/);
  assert.match(profilingPage, /const chartHeight = 34/);
  assert.match(profilingPage, /lg:grid-cols-2/);
  assert.match(profilingPage, /cpuActions/);
  assert.doesNotMatch(profilingPage, /Resource Trend|1s samples and 1min averages/);
  assert.doesNotMatch(profilingPage, /grid-cols-\[8rem_1fr\]/);
  assert.doesNotMatch(profilingPage, /resources\.cpu_percent|resources\.memory_size/);
  assert.match(profilingPage, /Top Functions/);
  assert.doesNotMatch(profilingPage, /ProfileMetricBars/);
  assert.match(profilingPage, /ResourceTrendChart/);
  assert.match(profilingPage, /\/api\/profile\?seconds=/);
  assert.match(profilingPage, /\/api\/resource/);
  assert.doesNotMatch(profilingPage, /RefreshCcw|Refresh/);
});

test("monitoring2 frontend avoids replay APIs", async () => {
  const sources = await Promise.all(
    [
      "../src/App.tsx",
      "../src/pages/AnalysisPage.tsx",
      "../src/pages/DebugPage.tsx",
      "../src/pages/LivePage.tsx",
      "../src/pages/ProgressPage.tsx",
      "../src/pages/ProfilingPage.tsx",
      "../src/hooks/useEngineTime.ts",
    ].map((path) => readFile(new URL(path, import.meta.url), "utf8")),
  );
  const combined = sources.join("\n");

  assert.doesNotMatch(combined, /\/api\/trace_range/);
  assert.doesNotMatch(combined, /\/api\/trace\?/);
  assert.doesNotMatch(combined, /\/api\/compnames/);
  assert.doesNotMatch(combined, /\/api\/compinfo/);
});
