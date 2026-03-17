import katex from "katex";
import "katex/dist/katex.min.css";
import type { ChatMessage } from "../../types/chat";

interface MessageBubbleProps {
  message: ChatMessage;
}

interface MarkdownToken {
  html: string;
  block: boolean;
}

const escapeHtml = (value: string): string =>
  value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#39;");

const renderMath = (expression: string, displayMode: boolean): string =>
  katex.renderToString(expression.trim(), {
    displayMode,
    throwOnError: false,
  });

const parseMarkdown = (input: string): string => {
  const tokens: MarkdownToken[] = [];

  const addToken = (html: string, block = false): string => {
    const tokenIndex = tokens.push({ html, block }) - 1;
    return `@@TOKEN_${tokenIndex}@@`;
  };

  let source = input;

  source = source.replace(/```([\s\S]*?)```/g, (_, code: string) => {
    const codeHtml = `<pre class="bg-dark text-light p-2 rounded mb-2" style="overflow-x:auto;"><code>${escapeHtml(
      code.trim(),
    )}</code></pre>`;
    return `\n${addToken(codeHtml, true)}\n`;
  });

  source = source.replace(/\$\$([\s\S]+?)\$\$/g, (_, expression: string) => {
    const html = renderMath(expression, true);
    return `\n${addToken(`<div class="my-2 overflow-auto">${html}</div>`, true)}\n`;
  });

  source = source.replace(/\$([^$\n]+?)\$/g, (_, expression: string) => addToken(renderMath(expression, false)));
  source = escapeHtml(source);

  const replaceTokens = (text: string): string =>
    text.replace(/@@TOKEN_(\d+)@@/g, (_, indexText: string) => {
      const index = Number(indexText);
      return tokens[index]?.html ?? "";
    });

  const applyInlineFormatting = (text: string): string => {
    let formatted = replaceTokens(text);
    formatted = formatted.replace(/`([^`]+)`/g, "<code>$1</code>");
    formatted = formatted.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    formatted = formatted.replace(/\*([^*]+)\*/g, "<em>$1</em>");
    return replaceTokens(formatted);
  };

  const lines = source.split(/\r?\n/);
  const htmlLines: string[] = [];
  let inList = false;

  const closeList = () => {
    if (!inList) return;
    htmlLines.push("</ul>");
    inList = false;
  };

  for (const rawLine of lines) {
    const trimmed = rawLine.trim();
    if (trimmed.length === 0) {
      closeList();
      continue;
    }

    const tokenMatch = trimmed.match(/^@@TOKEN_(\d+)@@$/);
    if (tokenMatch) {
      const tokenIndex = Number(tokenMatch[1]);
      const token = tokens[tokenIndex];
      if (token?.block) {
        closeList();
        htmlLines.push(token.html);
        continue;
      }
    }

    if (trimmed.startsWith("## ")) {
      closeList();
      htmlLines.push(`<h6 class="mb-1">${applyInlineFormatting(trimmed.slice(3))}</h6>`);
      continue;
    }

    if (trimmed.startsWith("# ")) {
      closeList();
      htmlLines.push(`<h5 class="mb-1">${applyInlineFormatting(trimmed.slice(2))}</h5>`);
      continue;
    }

    const listMatch = trimmed.match(/^[-*]\s+(.*)$/);
    if (listMatch) {
      if (!inList) {
        htmlLines.push('<ul class="mb-1 ps-3">');
        inList = true;
      }

      htmlLines.push(`<li>${applyInlineFormatting(listMatch[1])}</li>`);
      continue;
    }

    closeList();
    htmlLines.push(`<p class="mb-1">${applyInlineFormatting(trimmed)}</p>`);
  }

  closeList();
  return htmlLines.join("");
};

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
