import LiveComponentsPage from "./LiveComponentsPage";
import LiveDashboardPage from "./LiveDashboardPage";

export default function LivePage() {
  return (
    <div className="grid h-full grid-cols-[minmax(20rem,26rem)_1fr] gap-3 bg-slate-50 p-3">
      <LiveDashboardPage compact />
      <LiveComponentsPage embedded />
    </div>
  );
}
