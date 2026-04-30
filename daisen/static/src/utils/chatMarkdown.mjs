import katex from "katex";

const escapeHtml = (value) =>
  value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");

const renderMath = (expression, displayMode) =>
  katex.renderToString(expression.trim(), {
    displayMode,
    throwOnError: false,
  });

export const parseMarkdown = (input) => {
  const tokens = [];

  const addToken = (html, block = false) => {
    const tokenIndex = tokens.push({ html, block }) - 1;
    return `@@TOKEN_${tokenIndex}@@`;
  };

  let source = input;

  source = source.replace(/```([\s\S]*?)```/g, (_, code) => {
    const codeHtml = `<pre class="bg-dark text-light p-2 rounded mb-2" style="overflow-x:auto;"><code>${escapeHtml(
      code.trim(),
    )}</code></pre>`;
    return `\n${addToken(codeHtml, true)}\n`;
  });

  source = source.replace(/\$\$([\s\S]+?)\$\$/g, (_, expression) => {
    const html = renderMath(expression, true);
    return `\n${addToken(`<div class="my-2 overflow-auto">${html}</div>`, true)}\n`;
  });

  source = source.replace(/\\\[([\s\S]+?)\\\]/g, (_, expression) => {
    const html = renderMath(expression, true);
    return `\n${addToken(`<div class="my-2 overflow-auto">${html}</div>`, true)}\n`;
  });

  source = source.replace(/\\\(([\s\S]+?)\\\)/g, (_, expression) =>
    addToken(renderMath(expression, false)),
  );

  source = source.replace(/\$([^$\n]+?)\$/g, (_, expression) =>
    addToken(renderMath(expression, false)),
  );

  source = escapeHtml(source);

  const replaceTokens = (text) =>
    text.replace(/@@TOKEN_(\d+)@@/g, (_, indexText) => {
      const index = Number(indexText);
      return tokens[index]?.html ?? "";
    });

  const applyInlineFormatting = (text) => {
    let formatted = replaceTokens(text);
    formatted = formatted.replace(/`([^`]+)`/g, "<code>$1</code>");
    formatted = formatted.replace(/\*\*([^*]+)\*\*/g, "<strong>$1</strong>");
    formatted = formatted.replace(/\*([^*]+)\*/g, "<em>$1</em>");
    return replaceTokens(formatted);
  };

  const lines = source.split(/\r?\n/);
  const htmlLines = [];
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
