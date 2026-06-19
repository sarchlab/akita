import { useEffect, useState } from "react";
import { Link, Outlet } from "react-router-dom";
import { Bot } from "lucide-react";
import ChatPanel from "./chat/ChatPanel";
import { Button } from "./ui/button";
import { useRenderReadyOnNavigation } from "../hooks/useRenderReady";

export default function Layout() {
  useRenderReadyOnNavigation();
  const [chatOpen, setChatOpen] = useState(false);

  // Other parts of the app can still open the chat via this window event.
  useEffect(() => {
    const openHandler = () => setChatOpen(true);
    window.addEventListener("daisen:open-chat", openHandler);
    return () => window.removeEventListener("daisen:open-chat", openHandler);
  }, []);

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <nav className="daisen-top-nav">
        <Link to="/dashboard" className="daisen-brand">
          Daisen
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
