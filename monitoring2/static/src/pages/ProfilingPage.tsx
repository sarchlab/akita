import { useCallback, useEffect, useState } from "react";
import { Activity, RefreshCcw } from "lucide-react";
import { Button } from "../components/ui/button";

interface ResourceResponse {
  cpu_percent: number;
  memory_size: number;
}

interface ProfileSummary {
  samples: number;
  locations: number;
  functions: number;
}

function formatBytes(bytes: number | null | undefined) {
  if (typeof bytes !== "number" || !Number.isFinite(bytes)) {
    return "-";
  }

  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = bytes;
  let unitIndex = 0;

  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex += 1;
  }

  const digits = value >= 10 || unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(digits)} ${units[unitIndex]}`;
}

function useResourceUsage() {
  const [resources, setResources] = useState<ResourceResponse>({ cpu_percent: 0, memory_size: 0 });

  const refresh = useCallback(() => {
    fetch("/api/resource")
      .then((response) => (response.ok ? response.json() : null))
      .then((json: unknown) => {
        if (!json || typeof json !== "object") {
          return;
        }

        const resource = json as Partial<ResourceResponse>;
        setResources({
          cpu_percent: typeof resource.cpu_percent === "number" ? resource.cpu_percent : 0,
          memory_size: typeof resource.memory_size === "number" ? resource.memory_size : 0,
        });
      })
      .catch(() => {});
  }, []);

  useEffect(() => {
    refresh();
    const id = window.setInterval(refresh, 1500);
    return () => window.clearInterval(id);
  }, [refresh]);

  return { resources, refresh };
}

function summarizeProfile(profile: unknown): ProfileSummary {
  if (!profile || typeof profile !== "object") {
    return { samples: 0, locations: 0, functions: 0 };
  }

  const p = profile as Record<string, unknown>;
  const sample = Array.isArray(p.sample) ? p.sample : Array.isArray(p.Sample) ? p.Sample : [];
  const location = Array.isArray(p.location) ? p.location : Array.isArray(p.Location) ? p.Location : [];
  const fn = Array.isArray(p.function) ? p.function : Array.isArray(p.Function) ? p.Function : [];

  return {
    samples: sample.length,
    locations: location.length,
    functions: fn.length,
  };
}

export default function ProfilingPage() {
  const { resources, refresh } = useResourceUsage();
  const [profileStatus, setProfileStatus] = useState("");
  const [profileSummary, setProfileSummary] = useState<ProfileSummary | null>(null);

  const captureProfile = async () => {
    setProfileStatus("Capturing CPU profile...");
    try {
      const response = await fetch("/api/profile");
      if (!response.ok) {
        throw new Error(`${response.status} ${response.statusText}`);
      }

      const profile = await response.json();
      setProfileSummary(summarizeProfile(profile));
      setProfileStatus("Profile captured");
    } catch (err) {
      setProfileStatus(err instanceof Error ? err.message : "Profile capture failed");
    }
  };

  return (
    <div className="h-full overflow-auto bg-slate-50 p-4">
      <div className="mx-auto flex max-w-6xl flex-col gap-4">
        <header className="flex flex-wrap items-center gap-3 border-b bg-white px-4 py-3">
          <Activity className="h-5 w-5 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <h1 className="text-base font-semibold">Profiling</h1>
            <div className="text-xs text-muted-foreground">CPU, memory, and on-demand CPU profile capture</div>
          </div>
          <Button type="button" size="sm" variant="outline" onClick={refresh}>
            <RefreshCcw /> Refresh
          </Button>
          <Button type="button" size="sm" onClick={captureProfile}>
            <Activity /> Capture CPU Profile
          </Button>
        </header>

        <section className="grid gap-4 md:grid-cols-2">
          <div className="rounded border bg-white p-4">
            <div className="mb-3 text-sm font-semibold">Resource Usage</div>
            <dl className="grid grid-cols-[8rem_1fr] gap-y-3 text-sm">
              <dt className="text-muted-foreground">CPU</dt>
              <dd className="font-mono">{resources.cpu_percent.toFixed(1)}%</dd>
              <dt className="text-muted-foreground">RSS</dt>
              <dd className="font-mono">{formatBytes(resources.memory_size)}</dd>
            </dl>
          </div>

          <div className="rounded border bg-white p-4">
            <div className="mb-3 text-sm font-semibold">Latest CPU Profile</div>
            {profileStatus ? <div className="mb-3 text-sm text-muted-foreground">{profileStatus}</div> : null}
            {profileSummary ? (
              <dl className="grid grid-cols-[8rem_1fr] gap-y-3 text-sm">
                <dt className="text-muted-foreground">Samples</dt>
                <dd className="font-mono">{profileSummary.samples}</dd>
                <dt className="text-muted-foreground">Locations</dt>
                <dd className="font-mono">{profileSummary.locations}</dd>
                <dt className="text-muted-foreground">Functions</dt>
                <dd className="font-mono">{profileSummary.functions}</dd>
              </dl>
            ) : (
              <div className="text-sm text-muted-foreground">No profile captured yet.</div>
            )}
          </div>
        </section>
      </div>
    </div>
  );
}
