import MarkdownIt from "markdown-it";

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

export const renderChatMarkdown = (text) => md.render(text ?? "");
