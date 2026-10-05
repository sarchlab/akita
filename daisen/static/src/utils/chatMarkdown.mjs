import MarkdownIt from "markdown-it";
import { canonicalViewUrl } from "./viewState.mjs";

// DaisenBot chat messages are rendered as GitHub-flavored markdown: tables,
// lists, code fences, inline code, bold/italic, blockquotes, headings, and
// autolinked URLs. We delegate to markdown-it instead of hand-rolling a parser.
//
// Safety: the model output is untrusted (any user-configured provider), so we
// disable raw-HTML passthrough here (`html: false`) AND the caller sanitizes the
// generated HTML with DOMPurify (see MessageBubble) as defense in depth.
const md = new MarkdownIt({
  html: false, // never render raw HTML embedded in the markdown source
  linkify: true, // turn bare URLs into links
  breaks: true, // a single newline becomes <br>, matching chat expectations
});

// Open links in a new tab and drop the opener reference, since responses may
// link out to arbitrary documentation.
const defaultLinkOpen =
  md.renderer.rules.link_open ||
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));
md.renderer.rules.link_open = (tokens, idx, options, env, self) => {
  tokens[idx].attrSet("target", "_blank");
  tokens[idx].attrSet("rel", "noopener noreferrer");
  return defaultLinkOpen(tokens, idx, options, env, self);
};

// Daisen view evidence: a markdown image whose URL is a Daisen view path (e.g.
// `![what you see](/component?name=L2Cache&starttime=0&endtime=379102000)`) renders
// as an evidence figure — a thumbnail the reader can click to enlarge, plus a caption
// that links to the live view in a new tab. The thumbnail's `src` is filled in by
// MessageBubble after sanitize (from the agent's captured render, or lazily), so big
// data URLs stay out of the sanitized HTML. Non-view images fall through unchanged.
const defaultImage =
  md.renderer.rules.image ||
  ((tokens, idx, options, _env, self) => self.renderToken(tokens, idx, options));
md.renderer.rules.image = (tokens, idx, options, env, self) => {
  const token = tokens[idx];
  const viewUrl = canonicalViewUrl(token.attrGet("src") ?? "");
  if (!viewUrl) {
    return defaultImage(tokens, idx, options, env, self);
  }
  const altAttr = md.utils.escapeHtml(self.renderInlineAsText(token.children ?? [], options, env));
  const urlAttr = md.utils.escapeHtml(viewUrl);
  const caption = altAttr || "Open this view";
  return (
    `<span class="daisen-evidence-figure">` +
    `<img class="daisen-evidence" data-view-url="${urlAttr}" alt="${altAttr}">` +
    `<a class="daisen-evidence-link" data-view-url="${urlAttr}" href="${urlAttr}"` +
    ` target="_blank" rel="noopener noreferrer">${caption} ↗</a>` +
    `</span>`
  );
};

export const renderChatMarkdown = (text) => md.render(text ?? "");
