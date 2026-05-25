import { Navigate, Route, Routes } from "react-router-dom";
import Layout from "./components/Layout";
import AnalysisPage from "./pages/AnalysisPage";
import DebugPage from "./pages/DebugPage";
import LivePage from "./pages/LivePage";
import ProfilingPage from "./pages/ProfilingPage";
import ProgressPage from "./pages/ProgressPage";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<Navigate to="/progress" replace />} />
        <Route path="progress" element={<ProgressPage />} />
        <Route path="monitor" element={<LivePage />} />
        <Route path="analysis" element={<AnalysisPage />} />
        <Route path="debug" element={<DebugPage />} />
        <Route path="profiling" element={<ProfilingPage />} />
        <Route path="dashboard" element={<LivePage />} />
        <Route path="task" element={<LivePage />} />
        <Route path="component" element={<LivePage />} />
        <Route path="live" element={<LivePage />} />
        <Route path="live/dashboard" element={<LivePage />} />
        <Route path="live/components" element={<LivePage />} />
        <Route path="live/progress" element={<ProgressPage />} />
        <Route path="live/analysis" element={<AnalysisPage />} />
        <Route path="live/debug" element={<DebugPage />} />
        <Route path="live/profiling" element={<ProfilingPage />} />
        <Route path="*" element={<Navigate to="/progress" replace />} />
      </Route>
    </Routes>
  );
}
