import katex from "katex";

const escapeHtml = (value) =>
  value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");

export const renderMathToString = (expression, displayMode) =>
  katex.renderToString(expression.trim(), {
    displayMode,
    throwOnError: false,
  });

const renderMath = renderMathToString;

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

export function autoWrapMath(text) {
  return text.replace(
    /^(?!\\\[)([0-9\.\+\-\*/\(\)\s×÷]+=[0-9\.\+\-\*/\(\)\s×÷]+)(?!\\\])$/gm,
    "\\[$1\\]",
  );
}

export function convertMarkdownToHTML(text) {
  text = text.replace(/```html\n([\s\S]*?)```/g, (_match, code) => {
    let trimmed = code.replace(/^\s*\n+/, "").replace(/\n+\s*$/, "").replace(/(<br>\s*){1,}/g, "<br>");
    trimmed = trimmed.replace(/(<\/h[1-6]>|<\/hr>|<\/p>|<\/table>|<\/ul>|<\/ol>|<\/pre>|<\/div>|<\/span>)(<br>)+/g, "$1");
    trimmed = trimmed.replace(/(<br>\s*)+(<table)/g, "$2");
    trimmed = trimmed.replace(/^(<br>\s*)+/, "");
    return trimmed;
  });

  text = text.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
    const trimmed = code.replace(/^\s*\n+/, "").replace(/\n+\s*$/, "");
    const escaped = trimmed.replace(/</g, "&lt;").replace(/>/g, "&gt;");
    return `<pre class="code-block"><code${lang ? ` class="language-${lang}"` : ""}>${escaped}</code></pre>`;
  });

  text = text.replace(/`([^`]+)`/g, (_match, code) => {
    const escaped = code.replace(/</g, "&lt;").replace(/>/g, "&gt;");
    return `<code class="inline-code">${escaped}</code>`;
  });

  text = text.replace(/^### (.+)$/gm, (_match, p1) => `<h5>${p1}</h5>`);
  text = text.replace(/^## (.+)$/gm, (_match, p1) => `<h4>${p1}</h4>`);
  text = text.replace(/^# (.+)$/gm, (_match, p1) => `<h3>${p1}</h3>`);
  text = text.replace(/^-{3,}$/gm, () => "<hr>");
  text = text.replace(/\*\*(.+?)\*\*/g, (_match, p1) => `<b>${p1}</b>`);
  text = text.replace(/\*(.+?)\*/g, (_match, p1) => `<i>${p1}</i>`);
  text = text.replace(/\\\[(.+?)\\\]/gs, (_match, p1) => {
    const clean = p1.replace(/\\\[|\\\]/g, "").trim();
    return `<span class="math" data-display="block">${clean}</span>`;
  });
  text = text.replace(/\\\((.+?)\\\)/gs, (_match, p1) =>
    `<span class="math" data-display="inline">${p1}</span>`,
  );
  text = text.replace(/\n/g, "<br>");
  text = text.replace(/(<br>)*\\\](<br>)*/g, "");
  text = text.replace(/(<br>)*\\\[(<br>)*/g, "");
  text = text.replace(/(<br>\s*){2,}/g, "<br>");
  text = text.replace(/(<\/h[1-6]>|<\/hr>|<\/p>|<\/table>|<\/ul>|<\/ol>|<\/pre>|<\/div>|<\/span>)(<br>)+/g, "$1");
  text = text.replace(/(<br>\s*)+(<table)/g, "$2");
  text = text.replace(/^(<br>\s*)+/, "");
  return text;
}

export const renderChatMarkdown = (text) => convertMarkdownToHTML(autoWrapMath(text));

export function renderMathInElement(root) {
  root.querySelectorAll(".math").forEach((el) => {
    try {
      const tex = el.textContent || "";
      const displayMode = el.getAttribute("data-display") === "block";
      el.innerHTML = katex.renderToString(tex, { displayMode });
    } catch (e) {
      el.innerHTML = "<span style='color:red'>Invalid math</span>";
      console.log("KaTeX error:", e, "for tex:", el.textContent);
    }
  });
}
