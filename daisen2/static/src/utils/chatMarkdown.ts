import katex from "katex";
import "katex/dist/katex.min.css";

function escapeHtml(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

export function renderChatMarkdown(text: string): string {
  let html = text;
  html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_match, lang, code) => {
    return `<pre class="code-block"><code${lang ? ` class="language-${lang}"` : ""}>${escapeHtml(code.trim())}</code></pre>`;
  });
  html = html.replace(/`([^`]+)`/g, (_match, code) => `<code class="inline-code">${escapeHtml(code)}</code>`);
  html = html.replace(/^### (.+)$/gm, "<h5>$1</h5>");
  html = html.replace(/^## (.+)$/gm, "<h4>$1</h4>");
  html = html.replace(/^# (.+)$/gm, "<h3>$1</h3>");
  html = html.replace(/\*\*(.+?)\*\*/g, "<b>$1</b>");
  html = html.replace(/\*(.+?)\*/g, "<i>$1</i>");
  html = html.replace(/\$\$([\s\S]+?)\$\$/g, '<span class="math" data-display="block">$1</span>');
  html = html.replace(/\$([^$]+?)\$/g, '<span class="math" data-display="inline">$1</span>');
  html = html.replace(/\\\[([\s\S]+?)\\\]/g, '<span class="math" data-display="block">$1</span>');
  html = html.replace(/\\\(([\s\S]+?)\\\)/g, '<span class="math" data-display="inline">$1</span>');
  html = html.replace(/\n/g, "<br>");
  return html;
}

export function renderMathInElement(element: HTMLElement): void {
  element.querySelectorAll<HTMLElement>(".math").forEach((mathElement) => {
    try {
      const displayMode = mathElement.dataset.display === "block";
      mathElement.innerHTML = katex.renderToString(mathElement.textContent ?? "", { displayMode });
    } catch {
      mathElement.innerHTML = "<span style='color:red'>Invalid math</span>";
    }
  });
}
