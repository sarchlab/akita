import { useMemo, useState } from "react";
import { ArrowUp, File, Folder } from "lucide-react";
import hljs from "highlight.js/lib/core";
import go from "highlight.js/lib/languages/go";
import "highlight.js/styles/github.css";
import WidgetCard from "./WidgetCard";
import { useCodeLs, useCodeRead } from "../hooks/useCode";
import type { CodeEntry } from "../types/overview";

hljs.registerLanguage("go", go);

function formatBytes(n?: number): string {
  if (!n) return "";
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`;
  if (n >= 1 << 10) return `${(n / (1 << 10)).toFixed(1)} KB`;
  return `${n} B`;
}

interface CodeBrowserWidgetProps {
  expandHref?: string;
}

// CodeBrowserWidget browses the simulator source recorded in the trace: a
// directory listing that drills into folders and a viewer for file contents.
export default function CodeBrowserWidget({ expandHref }: CodeBrowserWidgetProps) {
  const [dir, setDir] = useState("");
  const [file, setFile] = useState<string | null>(null);
  const ls = useCodeLs(dir);
  const read = useCodeRead(file);

  const roots = ls.data?.roots ?? [];
  const atRoot = dir === "";

  const open = (entry: CodeEntry) => {
    const full = atRoot ? entry.name : `${dir}/${entry.name}`;
    if (entry.is_dir) {
      setDir(full);
      setFile(null);
    } else {
      setFile(full);
    }
  };

  const goUp = () => {
    if (file) {
      setFile(null);
      return;
    }
    if (atRoot) return;
    if (roots.includes(dir)) {
      setDir("");
    } else {
      setDir(dir.split("/").slice(0, -1).join("/"));
    }
  };

  const location = file ?? (atRoot ? "source roots" : dir);
  const canGoUp = file !== null || !atRoot;

  return (
    <WidgetCard
      title="Source code"
      expandHref={expandHref}
      contentClassName="flex flex-col p-0"
    >
      <div className="flex items-center gap-2 border-b px-3 py-1.5 text-xs">
        <button
          type="button"
          onClick={goUp}
          disabled={!canGoUp}
          className="flex items-center gap-1 rounded px-1.5 py-0.5 text-muted-foreground hover:bg-muted disabled:opacity-40"
        >
          <ArrowUp className="h-3.5 w-3.5" />
          Up
        </button>
        <span className="truncate font-mono text-muted-foreground" title={location}>
          {location}
        </span>
      </div>

      <div className="min-h-0 flex-1 overflow-auto">
        {file ? (
          <FileView
            loading={read.loading}
            error={read.error}
            content={read.data?.content ?? ""}
            path={file}
          />
        ) : (
          <DirView
            loading={ls.loading}
            error={ls.error}
            entries={ls.data?.entries ?? []}
            onOpen={open}
          />
        )}
      </div>
    </WidgetCard>
  );
}

function DirView({
  loading,
  error,
  entries,
  onOpen,
}: {
  loading: boolean;
  error: string | null;
  entries: CodeEntry[];
  onOpen: (e: CodeEntry) => void;
}) {
  if (loading) {
    return <div className="p-3 text-sm text-muted-foreground">Loading…</div>;
  }
  if (error) {
    return <div className="p-3 text-sm text-destructive">{error}</div>;
  }
  if (entries.length === 0) {
    return (
      <div className="p-3 text-sm text-muted-foreground">
        No source recorded in this trace.
      </div>
    );
  }
  return (
    <ul className="flex flex-col py-1">
      {entries.map((e) => (
        <li key={e.name}>
          <button
            type="button"
            onClick={() => onOpen(e)}
            className="flex w-full items-center gap-2 px-3 py-1 text-left text-xs hover:bg-muted"
          >
            {e.is_dir ? (
              <Folder className="h-3.5 w-3.5 shrink-0 text-primary" />
            ) : (
              <File className="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
            )}
            <span className="truncate font-mono">{e.name}</span>
            {!e.is_dir && e.size ? (
              <span className="ml-auto shrink-0 tabular-nums text-muted-foreground">
                {formatBytes(e.size)}
              </span>
            ) : null}
          </button>
        </li>
      ))}
    </ul>
  );
}

function FileView({
  loading,
  error,
  content,
  path,
}: {
  loading: boolean;
  error: string | null;
  content: string;
  path: string;
}) {
  const highlighted = useMemo(() => {
    if (!path.endsWith(".go")) return null;
    return hljs.highlight(content, { language: "go" }).value;
  }, [content, path]);

  if (loading) {
    return <div className="p-3 text-sm text-muted-foreground">Loading…</div>;
  }
  if (error) {
    return <div className="p-3 text-sm text-destructive">{error}</div>;
  }
  const lineCount = content.split("\n").length;
  return (
    <pre className="flex min-w-full text-xs leading-relaxed">
      <code className="select-none whitespace-pre border-r bg-muted/40 px-2 py-2 text-right text-muted-foreground">
        {Array.from({ length: lineCount }, (_, i) => `${i + 1}\n`)}
      </code>
      {highlighted ? (
        <code
          className="hljs whitespace-pre px-3 py-2 font-mono"
          dangerouslySetInnerHTML={{ __html: highlighted }}
        />
      ) : (
        <code className="whitespace-pre px-3 py-2 font-mono">{content}</code>
      )}
    </pre>
  );
}
