// Dev preview for the inline-evidence thumbnail's loading skeleton. It renders the
// real chat markup (the same classes MessageBubble produces) with the loading class
// applied statically, so it uses the actual compiled CSS and stays in the loading
// state — it never swaps to a captured image. Open at /preview.

// 1x1 transparent GIF, the same placeholder MessageBubble sets while a view renders,
// so the <img> shows the CSS skeleton instead of a broken-image icon.
const TRANSPARENT = "data:image/gif;base64,R0lGODlhAQABAAAAACH5BAEKAAEALAAAAAABAAEAAAICTAEAOw==";

function EvidenceFigure({ caption, state }: { caption: string; state: "loading" | "failed" }) {
  return (
    <span className="daisen-evidence-figure">
      <img className={`daisen-evidence daisen-evidence-${state}`} src={TRANSPARENT} alt={caption} />
      <a className="daisen-evidence-link" href="/dashboard" target="_blank" rel="noopener noreferrer">
        {caption} ↗
      </a>
    </span>
  );
}

export default function EvidencePreviewPage() {
  return (
    <div className="min-h-screen bg-background p-8 text-foreground">
      <div className="mx-auto max-w-xl space-y-8">
        <header className="space-y-1">
          <h1 className="text-lg font-semibold">DaisenBot — evidence thumbnail states</h1>
          <p className="text-sm text-muted-foreground">
            The chart-skeleton placeholder shown while a cited Daisen view renders off-screen.
            This page renders the real markup statically, so it stays in the loading state.
          </p>
        </header>

        <section className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">Loading — in an assistant chat bubble</div>
          <div className="chat-markdown w-fit max-w-full rounded-2xl bg-muted px-3 py-2 text-sm leading-relaxed text-foreground">
            <p>The view I rendered was:</p>
            <EvidenceFigure caption="AT dashboard widget" state="loading" />
          </div>
        </section>

        <section className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">Loading — on the page background</div>
          <div className="chat-markdown text-sm">
            <EvidenceFigure caption="L2Cache occupancy" state="loading" />
          </div>
        </section>

        <section className="space-y-2">
          <div className="text-xs font-medium text-muted-foreground">
            Failed (capture unavailable — thumbnail hidden, caption link kept)
          </div>
          <div className="chat-markdown w-fit max-w-full rounded-2xl bg-muted px-3 py-2 text-sm leading-relaxed text-foreground">
            <p>The view I rendered was:</p>
            <EvidenceFigure caption="MemCtrl timeline" state="failed" />
          </div>
        </section>

        <p className="text-xs text-muted-foreground">
          Dev preview at <code>/preview</code> — not linked from the app.
        </p>
      </div>
    </div>
  );
}
