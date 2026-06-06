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
  assert.match(layout, /AkitaRTM/);
  assert.doesNotMatch(layout, /Akita Monitor/);
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
  assert.match(page, /PROPERTY_REFRESH_INTERVAL_MS = 2000/);
  assert.match(page, /autoRefreshProperties/);
  assert.match(page, /checked=\{autoRefreshProperties\}/);
  assert.match(page, /setAutoRefreshProperties\(event\.target\.checked\)/);
  assert.match(page, /Refresh Properties/);
  assert.match(page, /disabled=\{!canToggle\}/);
  assert.match(page, /ChevronRight/);
  assert.match(page, /ChevronLeft/);
  assert.match(page, /ChevronDown/);
  assert.match(page, /Flag/);
  assert.match(page, /FlagOff/);
  assert.match(page, /grid-cols-\[minmax\(8rem,16rem\)_minmax\(10rem,1fr\)_minmax\(7rem,12rem\)_2\.25rem_2\.25rem\]/);
  assert.match(page, /className="contents text-left"/);
  assert.match(page, /tabular-nums/);
  assert.match(page, /justify-self-center/);
  assert.match(page, /INTEGER_KINDS/);
  assert.match(page, /FLOAT_KINDS/);
  assert.match(page, /MAP_KIND/);
  assert.match(page, /SLICE_KIND/);
  assert.match(page, /monitorSampleKind/);
  assert.match(page, /function nodeLength/);
  assert.match(page, /function valuePreview/);
  assert.match(page, /node\?\.k === SLICE_KIND/);
  assert.match(page, /child\?\.k === SLICE_KIND \|\| !expandable \? valuePreview\(child\) : ""/);
  assert.match(page, /SLICE_PAGE_SIZE = 50/);
  assert.match(page, /slice_offset/);
  assert.match(page, /slice_limit/);
  assert.match(page, /function SlicePagination/);
  assert.match(page, /onSlicePageChange/);
  assert.match(page, /addWatchedProperty/);
  assert.match(page, /removeWatchedProperty/);
  assert.match(page, /subscribeToWatchedProperties/);
  assert.match(page, /Monitor property/);
  assert.match(page, /ActionIcon/);
  assert.match(page, /aria-label=\{`\$\{actionLabel\}/);
  assert.doesNotMatch(page, /useEngineTime/);
  assert.doesNotMatch(page, /formatPicosecondsAsNanoseconds\(now\)/);
  assert.doesNotMatch(page, /Engine time/);
  assert.doesNotMatch(page, /\/api\/pause|\/api\/continue/);
  assert.doesNotMatch(page, /Open Field/);
  assert.doesNotMatch(page, /onDoubleClick/);
  assert.doesNotMatch(page, /grid h-full min-h-0 grid-rows-3 gap-3/);
  assert.doesNotMatch(page, /flex min-h-0 flex-col overflow-hidden rounded border bg-white/);
  assert.doesNotMatch(page, /\/api\/component\//);
  assert.match(page, /chooseComponent/);
  assert.doesNotMatch(page, /smartString\(now\)/);
  assert.match(smartValue, /formatPicosecondsAsNanoseconds/);
  assert.match(smartValue, /value \/ 1000/);
  assert.match(smartValue, /ns/);
  assert.doesNotMatch(page, /Start Tracing/);
  assert.doesNotMatch(page, /Stop Tracing/);
  assert.doesNotMatch(page, /Pause Tracing/);
  assert.doesNotMatch(page, /\/api\/trace\//);
  assert.doesNotMatch(page, /Tick Selected/);
  assert.doesNotMatch(page, /refreshComponents/);
  assert.doesNotMatch(page, /onClick=\{refreshComponents\}/);
  assert.doesNotMatch(page, /setInterval\(refresh/);
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
  const layout = await readFile(new URL("../src/components/Layout.tsx", import.meta.url), "utf8");
  const propertySamplesHook = await readFile(
    new URL("../src/hooks/usePropertyMonitoringSamples.ts", import.meta.url),
    "utf8",
  );
  const resourceUsageHook = await readFile(
    new URL("../src/hooks/useResourceUsageHistory.ts", import.meta.url),
    "utf8",
  );
  const progressPage = await readFile(new URL("../src/pages/ProgressPage.tsx", import.meta.url), "utf8");
  const profilingPage = await readFile(new URL("../src/pages/ProfilingPage.tsx", import.meta.url), "utf8");

  assert.doesNotMatch(livePage, /Buffer Level Analysis/);
  assert.doesNotMatch(livePage, /\/api\/hangdetector\/buffers/);
  assert.doesNotMatch(livePage, /monitorTab ===/);
  assert.doesNotMatch(analysisPage, /Gauge/);
  assert.doesNotMatch(analysisPage, /<h1 className="text-base font-semibold">Analysis<\/h1>/);
  assert.doesNotMatch(analysisPage, /border-b bg-white px-4 py-3/);
  assert.match(analysisPage, /type AnalysisTab = "properties" \| "buffers"/);
  assert.match(analysisPage, /role="tablist"/);
  assert.match(analysisPage, /aria-label="Analysis views"/);
  assert.match(analysisPage, /Property Monitoring/);
  assert.match(analysisPage, /Buffer Level Analysis/);
  assert.match(analysisPage, /type BufferSortMethod = "name" \| "level" \| "fullness"/);
  assert.match(analysisPage, /aria-selected=\{activeTab === "properties"\}/);
  assert.match(analysisPage, /aria-selected=\{activeTab === "buffers"\}/);
  assert.match(analysisPage, /activeTab === "properties"/);
  assert.match(analysisPage, /setActiveTab\("buffers"\)/);
  assert.match(analysisPage, /useBuffers\(bufferSortMethod, activeTab === "buffers", autoRefreshBuffers\)/);
  assert.match(analysisPage, /sortedBuffers/);
  assert.match(analysisPage, /setBufferSortMethod\("name"\)/);
  assert.match(analysisPage, /setBufferSortMethod\("level"\)/);
  assert.match(analysisPage, /setBufferSortMethod\("fullness"\)/);
  assert.match(analysisPage, /Sort by/);
  assert.match(analysisPage, /Name/);
  assert.match(analysisPage, /Buffer Level/);
  assert.match(analysisPage, /Fullness/);
  assert.match(analysisPage, /Auto refresh/);
  assert.match(analysisPage, /checked=\{autoRefreshBuffers\}/);
  assert.match(analysisPage, /setAutoRefreshBuffers\(event\.target\.checked\)/);
  assert.match(analysisPage, /usePropertyMonitoringSamples\(\)/);
  assert.doesNotMatch(analysisPage, /function usePropertySamples/);
  assert.match(layout, /PropertyMonitoringCollector/);
  assert.match(propertySamplesHook, /function collectPropertySamples/);
  assert.match(propertySamplesHook, /export function PropertyMonitoringCollector/);
  assert.match(propertySamplesHook, /export function usePropertyMonitoringSamples/);
  assert.match(propertySamplesHook, /subscribeToWatchedProperties/);
  assert.match(propertySamplesHook, /\/api\/field\//);
  assert.match(propertySamplesHook, /\/api\/now/);
  assert.match(propertySamplesHook, /PROPERTY_SAMPLE_INTERVAL_MS = 1000/);
  assert.match(propertySamplesHook, /MAX_PROPERTY_SAMPLES = 120/);
  assert.match(propertySamplesHook, /watchedSnapshotValue/);
  assert.doesNotMatch(analysisPage, /function usePropertySamples\(properties: WatchedProperty\[\], enabled: boolean\)/);
  assert.doesNotMatch(analysisPage, /Aggregate Buffer Level/);
  assert.doesNotMatch(analysisPage, /totalPercent/);
  assert.doesNotMatch(analysisPage, /totals/);
  assert.match(analysisPage, /\/api\/hangdetector\/buffers/);
  assert.match(analysisPage, /limit=256/);
  assert.match(analysisPage, /PropertyChart/);
  assert.match(propertySamplesHook, /sampleKind === "count"/);
  assert.doesNotMatch(analysisPage, /typeof value === "boolean"/);
  assert.doesNotMatch(analysisPage, /Number\(value\)/);
  assert.match(analysisPage, /getWatchedProperties/);
  assert.match(analysisPage, /Waiting for numeric samples/);
  assert.match(analysisPage, /repeat\(auto-fill,minmax\(11rem,1fr\)\)/);
  assert.match(analysisPage, /bufferFillClass/);
  assert.doesNotMatch(analysisPage, /RefreshCcw/);
  assert.doesNotMatch(analysisPage, /border-b p-4 last:border-b-0/);
  assert.match(progressPage, /Execution/);
  assert.match(progressPage, /\/api\/progress/);
  assert.match(progressPage, /\/api\/engine\/state/);
  assert.match(progressPage, /\/api\/pause/);
  assert.match(progressPage, /\/api\/continue/);
  assert.doesNotMatch(progressPage, /ListChecks/);
  assert.doesNotMatch(progressPage, /<h1 className="text-base font-semibold">Execution<\/h1>/);
  assert.doesNotMatch(progressPage, /State <span/);
  assert.doesNotMatch(progressPage, /setControlStatus/);
  assert.match(progressPage, /border-amber-500/);
  assert.match(progressPage, /border-emerald-500/);
  assert.match(progressPage, /Current Virtual Time/);
  assert.match(progressPage, /useEngineTime\(500\)/);
  assert.match(progressPage, /formatPicosecondsAsNanoseconds\(now\)/);
  assert.match(progressPage, /ControlActionIcon/);
  assert.match(progressPage, /isPaused \? Play : Pause/);
  assert.match(progressPage, /post\(isPaused \? "\/api\/continue" : "\/api\/pause"\)/);
  assert.match(progressPage, /Pause simulation/);
  assert.match(progressPage, /Continue simulation/);
  assert.match(progressPage, /text-xl font-semibold/);
  assert.doesNotMatch(progressPage, /text-3xl font-semibold/);
  assert.doesNotMatch(progressPage, /RefreshCcw/);
  assert.doesNotMatch(progressPage, /totals/);
  assert.match(progressPage, /\/api\/execution\/info/);
  assert.match(progressPage, /ExecutionInfoEntry/);
  assert.match(progressPage, /Execution Info/);
  assert.match(progressPage, /Recorded in exec_info/);
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
  assert.match(profilingPage, /\{profileKindLabel\} Call Graph/);
  assert.match(profilingPage, /activeProfileTab/);
  assert.match(profilingPage, /Profile views/);
  // Heap profiling parity: capture button, endpoint, and inuse/alloc selector.
  assert.match(profilingPage, /Capture Heap Profile/);
  assert.match(profilingPage, /\/api\/heap/);
  assert.match(profilingPage, /HEAP_SAMPLE_TYPES/);
  assert.match(profilingPage, /heapSampleType/);
  assert.match(profilingPage, /memoryActions/);
  assert.match(profilingPage, /inuse_space/);
  // Incremental heap profiling: scratch/incremental mode gated on a baseline.
  assert.match(profilingPage, /From scratch/);
  assert.match(profilingPage, /Incremental/);
  assert.match(profilingPage, /hasHeapBaseline/);
  assert.match(profilingPage, /mode=\$\{incremental \? "incremental" : "scratch"\}/);
  assert.match(profilingPage, /disabled=\{!hasHeapBaseline\}/);
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
  assert.match(layout, /ResourceUsageCollector/);
  assert.match(profilingPage, /useResourceUsageHistory\(\)/);
  assert.doesNotMatch(profilingPage, /function useResourceUsage/);
  assert.match(resourceUsageHook, /function collectResourceUsage/);
  assert.match(resourceUsageHook, /export function ResourceUsageCollector/);
  assert.match(resourceUsageHook, /export function useResourceUsageHistory/);
  assert.match(resourceUsageHook, /\/api\/resource/);
  assert.match(resourceUsageHook, /RESOURCE_SAMPLE_INTERVAL_MS = 1000/);
  assert.match(resourceUsageHook, /MAX_SECOND_SAMPLES = 60/);
  assert.match(resourceUsageHook, /MAX_MINUTE_SAMPLES = 60/);
  assert.match(profilingPage, /MAX_SECOND_SAMPLES/);
  assert.match(profilingPage, /MAX_MINUTE_SAMPLES/);
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
  assert.doesNotMatch(profilingPage, /\/api\/resource/);
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
