import { Play, Pause, StepForward } from "lucide-react";
import { Button } from "../components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { useEngineTime } from "../hooks/useEngineTime";
import { smartString } from "../utils/smartValue";

function post(path: string) {
  void fetch(path, { method: "POST" });
}

export default function LiveDashboardPage({ compact = false }: { compact?: boolean }) {
  const now = useEngineTime();
  return (
    <div className={compact ? "min-h-0" : "h-full overflow-auto bg-slate-50 p-4"}>
      <Card className="rounded-md shadow-none">
        <CardHeader>
          <CardTitle>Live Execution</CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="text-sm text-muted-foreground">Engine time</div>
          <div className="text-2xl font-semibold">{now == null ? "-" : smartString(now)}</div>
          <div className="flex flex-wrap gap-2">
            <Button type="button" onClick={() => post("/api/continue")}>
              <Play /> Continue
            </Button>
            <Button type="button" variant="outline" onClick={() => post("/api/pause")}>
              <Pause /> Pause
            </Button>
            <Button type="button" variant="secondary" onClick={() => post("/api/tick/1")}>
              <StepForward /> Tick
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
