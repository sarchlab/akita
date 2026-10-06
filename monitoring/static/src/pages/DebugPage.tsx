import { useCallback, useEffect, useMemo, useState } from "react";
import { Bug, RefreshCcw, Search, StepForward } from "lucide-react";
import { Button } from "../components/ui/button";
import { Input } from "../components/ui/input";

function useComponentNames() {
  const [components, setComponents] = useState<string[]>([]);

  const refresh = useCallback(() => {
    fetch("/api/list_components")
      .then((response) => (response.ok ? response.json() : []))
      .then((json: unknown) => {
        setComponents(Array.isArray(json) ? json.filter((item) => typeof item === "string") : []);
      })
      .catch(() => setComponents([]));
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 2000);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { components, refresh };
}

async function post(path: string) {
  const response = await fetch(path, { method: "POST" });
  if (!response.ok) {
    throw new Error(`${response.status} ${response.statusText}`);
  }
}

export default function DebugPage() {
  const { components, refresh } = useComponentNames();
  const [filter, setFilter] = useState("");
  const [selectedComponent, setSelectedComponent] = useState("");
  const [status, setStatus] = useState("");

  useEffect(() => {
    if (!selectedComponent && components.length) {
      setSelectedComponent(components[0]);
    }
  }, [components, selectedComponent]);

  const visibleComponents = useMemo(() => {
    if (!filter) {
      return components;
    }

    return components.filter((component) => component.includes(filter));
  }, [components, filter]);

  const scheduleTick = async (component: string) => {
    if (!component) {
      return;
    }

    setStatus(`Scheduling tick for ${component}...`);
    try {
      await post(`/api/tick/${encodeURIComponent(component)}`);
      setStatus(`Scheduled tick for ${component}`);
    } catch (err) {
      setStatus(err instanceof Error ? err.message : "Failed to schedule tick");
    }
  };

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <Bug className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Debug</h1>
            <div className="text-xs text-muted-foreground">{components.length} components</div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            <RefreshCcw /> Refresh
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={!selectedComponent}
            onClick={() => scheduleTick(selectedComponent)}
          >
            <StepForward /> Tick Selected
          </Button>
        </header>

        <section className="border bg-white">
          <div className="flex flex-wrap items-center gap-3 border-b p-3">
            <Search className="h-4 w-4 text-muted-foreground" />
            <Input
              className="max-w-md"
              value={filter}
              placeholder="Filter components"
              onChange={(event) => setFilter(event.target.value)}
            />
            {status ? <div className="ml-auto text-xs text-muted-foreground">{status}</div> : null}
          </div>

          {visibleComponents.length ? (
            <div className="divide-y">
              {visibleComponents.map((component) => (
                <div
                  key={component}
                  className={`grid grid-cols-[minmax(0,1fr)_auto] items-center gap-3 px-4 py-3 ${
                    selectedComponent === component ? "bg-primary/5" : "bg-white"
                  }`}
                >
                  <button
                    type="button"
                    className={`min-w-0 truncate text-left text-sm ${
                      selectedComponent === component ? "font-semibold text-primary" : "font-medium"
                    }`}
                    onClick={() => setSelectedComponent(component)}
                  >
                    {component}
                  </button>
                  <Button type="button" size="sm" variant="outline" onClick={() => scheduleTick(component)}>
                    <StepForward /> Schedule Tick
                  </Button>
                </div>
              ))}
            </div>
          ) : (
            <div className="p-10 text-center text-sm text-muted-foreground">No components available.</div>
          )}
        </section>
      </div>
    </div>
  );
}
