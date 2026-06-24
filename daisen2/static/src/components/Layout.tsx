import { useEffect, useState } from "react";
import { Link, Outlet } from "react-router-dom";
import { Bot } from "lucide-react";
import ChatPanel from "./chat/ChatPanel";
import { Button } from "./ui/button";
import { useRenderReadyOnNavigation } from "../hooks/useRenderReady";
import { useSimInfo } from "../hooks/useSimInfo";

export default function Layout() {
  useRenderReadyOnNavigation();
  const [chatOpen, setChatOpen] = useState(false);
  // The command that produced this trace (from the exec_info table), shown as a
  // subtitle under the brand so it is always clear which run is being viewed.
  const { data: simInfo } = useSimInfo();
  const command = simInfo?.find((entry) => entry.property === "Command")?.value;

  // Other parts of the app can still open the chat via this window event.
  useEffect(() => {
    const openHandler = () => setChatOpen(true);
    window.addEventListener("daisen:open-chat", openHandler);
    return () => window.removeEventListener("daisen:open-chat", openHandler);
  }, []);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <nav className="daisen-top-nav">
        <Link to="/" className="daisen-brand">
          <span className="leading-none">Daisen</span>
          {command ? (
            <span
              className="mt-0.5 max-w-[45vw] truncate font-mono text-[11px] font-normal leading-none text-slate-400"
              title={command}
            >
              {command}
            </span>
          ) : null}
        </Link>
        <Button
          type="button"
          size="sm"
          variant={chatOpen ? "secondary" : "default"}
          className="ml-auto"
          onClick={() => setChatOpen((value) => !value)}
        >
          <Bot />
          Daisen Bot
        </Button>
      </nav>
      {/* The chat docks beside the content and narrows it (no overlay), so the
          visualizations reflow to the new width via their ResizeObservers. The
          panel stays mounted while closed (hidden via CSS) so closing and
          reopening does not discard the conversation or an in-flight answer. */}
      <div className="flex min-h-0 flex-1">
        <main className="daisen-main min-w-0 flex-1">
          <Outlet />
        </main>
        <ChatPanel open={chatOpen} />
      </div>
    </div>
  );
}
