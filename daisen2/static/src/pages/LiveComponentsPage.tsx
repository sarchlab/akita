import { useEffect, useState } from "react";
import { Card, CardContent, CardHeader, CardTitle } from "../components/ui/card";
import { Input } from "../components/ui/input";

export default function LiveComponentsPage({ embedded = false }: { embedded?: boolean }) {
  const [components, setComponents] = useState<string[]>([]);
  const [filter, setFilter] = useState("");

  useEffect(() => {
    fetch("/api/list_components")
      .then((response) => (response.ok ? response.json() : []))
      .then((json: string[]) => setComponents(Array.isArray(json) ? json : []))
      .catch(() => setComponents([]));
  }, []);

  const visible = components.filter((component) => component.includes(filter));

  return (
    <div className={embedded ? "min-h-0" : "h-full overflow-auto bg-slate-50 p-4"}>
      <Card className="h-full rounded-md shadow-none">
        <CardHeader>
          <CardTitle>Component Inspector</CardTitle>
        </CardHeader>
        <CardContent className="space-y-3">
          <Input placeholder="Filter components" value={filter} onChange={(event) => setFilter(event.target.value)} />
          <div className="max-h-[calc(100vh-14rem)] overflow-auto rounded-md border bg-white">
            {visible.length ? (
              visible.map((component) => (
                <div key={component} className="border-b px-3 py-2 text-sm last:border-b-0">
                  {component}
                </div>
              ))
            ) : (
              <div className="p-6 text-center text-sm text-muted-foreground">No live components available.</div>
            )}
          </div>
        </CardContent>
      </Card>
    </div>
  );
}
