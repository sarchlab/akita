import { Navigate, Route, Routes } from "react-router-dom";
import Layout from "./components/Layout";
import ChartsPage from "./pages/ChartsPage";
import OverviewPage from "./pages/OverviewPage";
import QueuePage from "./pages/QueuePage";
import ResultsPage from "./pages/ResultsPage";
import SessionsPage from "./pages/SessionsPage";

export default function App() {
  return (
    <Routes>
      <Route element={<Layout />}>
        <Route index element={<OverviewPage />} />
        <Route path="sessions" element={<SessionsPage />} />
        <Route path="results" element={<ResultsPage />} />
        <Route path="charts" element={<ChartsPage />} />
        <Route path="queue" element={<QueuePage />} />
        <Route path="*" element={<Navigate to="/" replace />} />
      </Route>
    </Routes>
  );
}
