import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("monitoring2 presents one monitor surface without product tabs", async () => {
  const app = await readFile(new URL("../src/App.tsx", import.meta.url), "utf8");
  const layout = await readFile(new URL("../src/components/Layout.tsx", import.meta.url), "utf8");
  const page = await readFile(new URL("../src/pages/LivePage.tsx", import.meta.url), "utf8");

  assert.match(app, /<Route index element={<Navigate to="\/progress" replace/);
  assert.match(app, /path="progress" element={<ProgressPage/);
  assert.match(app, /path="monitor" element={<LivePage/);
  assert.match(app, /path="analysis" element={<AnalysisPage/);
  assert.match(app, /path="debug" element={<DebugPage/);
  assert.match(app, /path="profiling" element={<ProfilingPage/);
  assert.match(app, /path="task" element={<LivePage/);
  assert.match(app, /path="component" element={<LivePage/);
  assert.match(app, /path="dashboard" element={<LivePage/);
  assert.match(layout, /Progress[\s\S]*Monitor[\s\S]*Analysis[\s\S]*Debug[\s\S]*Profiling/);
  assert.match(layout, /Monitor/);
  assert.match(layout, /Progress/);
  assert.match(layout, /Analysis/);
  assert.match(layout, /Debug/);
  assert.match(layout, /Profiling/);
  assert.match(layout, /role="tablist"/);
  assert.doesNotMatch(layout, /Dashboard|Task|Component/);
  assert.doesNotMatch(page, /Live Execution/);
});

test("monitoring2 page supports component selection and tracing controls", async () => {
  const page = await readFile(new URL("../src/pages/LivePage.tsx", import.meta.url), "utf8");

  assert.match(page, /\/api\/list_components/);
  assert.match(page, /\/api\/component\//);
  assert.match(page, /\/api\/field\//);
  assert.match(page, /chooseComponent/);
  assert.match(page, /Start Tracing/);
  assert.match(page, /Pause Tracing/);
  assert.match(page, /\/api\/trace\/start/);
  assert.match(page, /\/api\/trace\/end/);
  assert.match(page, /\/api\/trace\/is_tracing/);
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
  assert.match(progressPage, /Progress/);
  assert.match(progressPage, /\/api\/progress/);
  assert.match(profilingPage, /Profiling/);
  assert.match(profilingPage, /Capture CPU Profile/);
  assert.match(profilingPage, /CPU Call Graph/);
  assert.match(profilingPage, /CallGraph/);
  assert.match(profilingPage, /Resource Trend/);
  assert.match(profilingPage, /Top Functions/);
  assert.match(profilingPage, /ProfileMetricBars/);
  assert.match(profilingPage, /ResourceTrendChart/);
  assert.match(profilingPage, /\/api\/profile/);
  assert.match(profilingPage, /\/api\/resource/);
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
