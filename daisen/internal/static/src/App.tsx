import { useEffect } from "react";
import { Navigate, Route, Routes, useLocation } from "react-router-dom";
import Layout from "./components/Layout";
import MainPage from "./pages/MainPage";
import WidgetPage from "./pages/WidgetPage";
import DashboardPage from "./pages/DashboardPage";
import TaskChartPage from "./pages/TaskChartPage";
import ComponentPage from "./pages/ComponentPage";
import ResourcePage from "./pages/ResourcePage";

// Redirect to the canonical /dashboard while preserving any query state, so
// shared/back-compat links like /dashboard?widget=…&starttime=… are not discarded.
function RedirectToDashboard() {
  const { search } = useLocation();
  return <Navigate to={{ pathname: "/dashboard", search }} replace />;
}

// Block the browser from zooming the whole page, so a trackpad pinch (delivered
// as ctrl+wheel) or a Safari pinch gesture zooms inside a chart — never the page.
// Touchscreen pinch is already covered by `touch-action: none` on the body.
function usePreventPageZoom() {
  useEffect(() => {
    const onWheel = (event: WheelEvent) => {
      if (event.ctrlKey) event.preventDefault();
    };
    const onGesture = (event: Event) => event.preventDefault();
    // Non-passive so preventDefault is honored; the charts' own zoom handlers
    // still run, this only suppresses the browser's page zoom on top.
    window.addEventListener("wheel", onWheel, { passive: false });
    window.addEventListener("gesturestart", onGesture, { passive: false });
    window.addEventListener("gesturechange", onGesture, { passive: false });
    window.addEventListener("gestureend", onGesture, { passive: false });
    return () => {
      window.removeEventListener("wheel", onWheel);
      window.removeEventListener("gesturestart", onGesture);
      window.removeEventListener("gesturechange", onGesture);
      window.removeEventListener("gestureend", onGesture);
    };
  }, []);
}

export default function App() {
  usePreventPageZoom();
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<MainPage />} />
        <Route path="view/:widget" element={<WidgetPage />} />
        <Route path="dashboard" element={<DashboardPage />} />
        <Route path="task" element={<TaskChartPage />} />
        <Route path="component" element={<ComponentPage />} />
        <Route path="resource" element={<ResourcePage />} />
        <Route path="*" element={<RedirectToDashboard />} />
      </Route>
    </Routes>
  );
}
