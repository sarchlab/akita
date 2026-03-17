import "katex/dist/katex.min.css";
import type { ChatMessage } from "../../types/chat";
import { parseMarkdown } from "../../utils/chatMarkdown";

interface MessageBubbleProps {
  message: ChatMessage;
}

const bubbleClassByRole: Record<ChatMessage["role"], string> = {
  user: "bg-primary text-white",
  assistant: "bg-light border",
  system: "bg-warning-subtle border",
};

export default function MessageBubble({ message }: MessageBubbleProps) {
  const isUser = message.role === "user";

  return (
    <div className={`d-flex mb-2 ${isUser ? "justify-content-end" : "justify-content-start"}`}>
      <div
        className={`rounded px-3 py-2 ${bubbleClassByRole[message.role]}`}
        style={{ maxWidth: "90%", overflowWrap: "anywhere" }}
      >
        {message.content.map((content, index) => {
          if (content.type === "image_url") {
            return (
              <img
                key={`${message.role}-img-${index}`}
                alt="Uploaded"
                className="img-fluid rounded"
                src={content.image_url.url}
                style={{ maxHeight: "240px" }}
              />
            );
          }

          return (
            <div
              key={`${message.role}-text-${index}`}
              className={index > 0 ? "mt-2" : undefined}
              dangerouslySetInnerHTML={{ __html: parseMarkdown(content.text) }}
            />
          );
        })}
      </div>
    </div>
  );
}
