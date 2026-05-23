import { Link, Outlet } from "react-router-dom";
import { Bot } from "lucide-react";
import ChatPanel from "./chat/ChatPanel";
import { Button } from "./ui/button";

export default function Layout() {
  return (
    <div className="flex h-full flex-col overflow-hidden">
      <nav className="daisen-top-nav">
        <Link to="/" className="daisen-brand">
          Daisen
        </Link>
        <Button
          type="button"
          size="sm"
          className="ml-auto"
          onClick={() => window.dispatchEvent(new CustomEvent("daisen:open-chat"))}
        >
          <Bot />
          Daisen Bot
        </Button>
      </nav>
      <main className="daisen-main">
        <Outlet />
      </main>
      <ChatPanel />
    </div>
  );
}
