import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import Layout from "./components/Layout";
import DashboardPage from "./pages/DashboardPage";
import TaskChartPage from "./pages/TaskChartPage";
import ComponentPage from "./pages/ComponentPage";

// Redirect to the canonical /dashboard while preserving any query state, so
// shared/back-compat root links like /?widget=…&starttime=… are not discarded.
function RedirectToDashboard() {
  const { search } = useLocation();
  return <Navigate to={{ pathname: "/dashboard", search }} replace />;
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<RedirectToDashboard />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="task" element={<TaskChartPage />} />
        <Route path="component" element={<ComponentPage />} />
        <Route path="*" element={<RedirectToDashboard />} />
      </Route>
    </Routes>
  );
}
