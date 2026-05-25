import { Navigate, Route, Routes } from "react-router-dom";
import Layout from "./components/Layout";
import LivePage from "./pages/LivePage";
import ProfilingPage from "./pages/ProfilingPage";
import ProgressPage from "./pages/ProgressPage";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<LivePage />} />
        <Route path="dashboard" element={<LivePage />} />
        <Route path="task" element={<LivePage />} />
        <Route path="component" element={<LivePage />} />
        <Route path="live" element={<LivePage />} />
        <Route path="live/dashboard" element={<LivePage />} />
        <Route path="live/components" element={<LivePage />} />
        <Route path="progress" element={<ProgressPage />} />
        <Route path="profiling" element={<ProfilingPage />} />
        <Route path="live/progress" element={<ProgressPage />} />
        <Route path="live/profiling" element={<ProfilingPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
