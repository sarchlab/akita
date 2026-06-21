import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import Layout from "./components/Layout";
import MainPage from "./pages/MainPage";
import WidgetPage from "./pages/WidgetPage";
import DashboardPage from "./pages/DashboardPage";
import TaskChartPage from "./pages/TaskChartPage";
import ComponentPage from "./pages/ComponentPage";

// Redirect to the canonical /dashboard while preserving any query state, so
// shared/back-compat links like /dashboard?widget=…&starttime=… are not discarded.
function RedirectToDashboard() {
  const { search } = useLocation();
  return <Navigate to={{ pathname: "/dashboard", search }} replace />;
}

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<MainPage />} />
        <Route path="view/:widget" element={<WidgetPage />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="task" element={<TaskChartPage />} />
        <Route path="component" element={<ComponentPage />} />
        <Route path="*" element={<RedirectToDashboard />} />
      </Route>
    </Routes>
  );
}
