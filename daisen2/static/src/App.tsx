import { Routes, Route, Navigate } from "react-router";
import Layout from "./components/Layout";
import DashboardPage from "./pages/DashboardPage";
import TaskChartPage from "./pages/TaskChartPage";
import ComponentPage from "./pages/ComponentPage";
import LiveComponentsPage from "./pages/LiveComponentsPage";
import LiveDashboardPage from "./pages/LiveDashboardPage";
import LivePage from "./pages/LivePage";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<DashboardPage />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="task" element={<TaskChartPage />} />
        <Route path="component" element={<ComponentPage />} />
        <Route path="live" element={<LivePage />} />
        <Route path="live/components" element={<LiveComponentsPage />} />
        <Route path="live/dashboard" element={<LiveDashboardPage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
