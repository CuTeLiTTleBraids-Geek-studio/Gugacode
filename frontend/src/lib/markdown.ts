import { marked } from "marked";
import DOMPurify from "dompurify";
import hljs from "highlight.js/lib/common";
// 额外注册 common 包未包含但常用的语言，覆盖更多开发场景。
import tsx from "highlight.js/lib/languages/typescript";
import jsx from "highlight.js/lib/languages/javascript";
import dockerfile from "highlight.js/lib/languages/dockerfile";
import nginx from "highlight.js/lib/languages/nginx";
import protobuf from "highlight.js/lib/languages/protobuf";
import scala from "highlight.js/lib/languages/scala";
import groovy from "highlight.js/lib/languages/groovy";
import dart from "highlight.js/lib/languages/dart";
import elixir from "highlight.js/lib/languages/elixir";
import haskell from "highlight.js/lib/languages/haskell";
import clojure from "highlight.js/lib/languages/clojure";
import vim from "highlight.js/lib/languages/vim";
import powershell from "highlight.js/lib/languages/powershell";
import ocaml from "highlight.js/lib/languages/ocaml";
import erlang from "highlight.js/lib/languages/erlang";
import { translate } from "@/lib/i18n";

// Register extra languages beyond the common set.
hljs.registerLanguage("tsx", tsx);
hljs.registerLanguage("jsx", jsx);
hljs.registerLanguage("dockerfile", dockerfile);
hljs.registerLanguage("nginx", nginx);
hljs.registerLanguage("protobuf", protobuf);
hljs.registerLanguage("scala", scala);
hljs.registerLanguage("groovy", groovy);
hljs.registerLanguage("dart", dart);
hljs.registerLanguage("elixir", elixir);
hljs.registerLanguage("haskell", haskell);
hljs.registerLanguage("clojure", clojure);
hljs.registerLanguage("vim", vim);
hljs.registerLanguage("powershell", powershell);
hljs.registerLanguage("ocaml", ocaml);
hljs.registerLanguage("erlang", erlang);

// Markdown 代码围栏中常用的别名 → hljs 注册名映射。
// marked 会把 ```ts 翻译成 class="language-ts"，但 hljs 注册名是
// "typescript"。"aliases" 字段 hljs 内部已注册部分，但为了兼容更多
// 简写（如 golang/py/sh），这里显式补全映射。
const LANG_ALIASES: Record<string, string> = {
  ts: "typescript",
  tsx: "tsx",
  js: "javascript",
  jsx: "jsx",
  mjs: "javascript",
  cjs: "javascript",
  es: "javascript",
  es6: "javascript",
  go: "go",
  golang: "go",
  py: "python",
  python3: "python",
  rb: "ruby",
  rs: "rust",
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  shell: "shell",
  ps: "powershell",
  ps1: "powershell",
  pwsh: "powershell",
  yml: "yaml",
  md: "markdown",
  markdown: "markdown",
  cplusplus: "cpp",
  cc: "cpp",
  h: "cpp",
  hpp: "cpp",
  cs: "csharp",
  fs: "fsharp",
  fsharp: "fsharp",
  kt: "kotlin",
  kts: "kotlin",
  scala: "scala",
  sc: "scala",
  groovy: "groovy",
  gradle: "groovy",
  dart: "dart",
  elixir: "elixir",
  ex: "elixir",
  exs: "elixir",
  hs: "haskell",
  clj: "clojure",
  cljs: "clojure",
  edn: "clojure",
  ml: "ocaml",
  erl: "erlang",
  vim: "vim",
  viml: "vim",
  dockerfile: "dockerfile",
  docker: "dockerfile",
  proto: "protobuf",
  protobuf: "protobuf",
  nginx: "nginx",
  conf: "nginx",
  ini: "ini",
  toml: "ini",
  tex: "latex",
  latex: "latex",
  html: "xml",
  xml: "xml",
  svg: "xml",
  rss: "xml",
  plist: "xml",
  sql: "sql",
  mysql: "sql",
  postgres: "sql",
  postgresql: "sql",
  psql: "sql",
  graphql: "graphql",
  gql: "graphql",
  wasm: "wasm",
  wat: "wasm",
  lua: "lua",
  make: "makefile",
  makefile: "makefile",
  cmake: "makefile",
  diff: "diff",
  patch: "diff",
  plaintext: "plaintext",
  text: "plaintext",
  txt: "plaintext",
  log: "plaintext",
};

function resolveLanguage(lang: string): string {
  const lower = lang.toLowerCase();
  return LANG_ALIASES[lower] ?? lower;
}

// Configure marked once
marked.setOptions({
  gfm: true,
  breaks: false,
});

/**
 * Applies highlight.js syntax highlighting to all `<pre><code>` blocks in the
 * HTML string. Uses the language-XXX class produced by marked to pick the
 * language; falls back to auto-detection when no class is present or the
 * language is unknown. Returns the HTML with highlighted code blocks.
 *
 * Inline `<code>` (not inside `<pre>`) is left untouched.
 */
function highlightCodeBlocks(html: string): string {
  const parser = new DOMParser();
  const doc = parser.parseFromString(html, "text/html");
  const codeBlocks = doc.querySelectorAll("pre > code");
  if (codeBlocks.length === 0) return html;
  codeBlocks.forEach((codeEl) => {
    const langMatch = codeEl.className.match(/language-([\w-]+)/);
    const rawLang = langMatch?.[1] || "";
    const lang = rawLang ? resolveLanguage(rawLang) : "";
    const code = codeEl.textContent || "";
    let highlighted: string;
    try {
      if (lang && hljs.getLanguage(lang)) {
        highlighted = hljs.highlight(code, { language: lang }).value;
      } else {
        highlighted = hljs.highlightAuto(code).value;
      }
    } catch {
      highlighted = code;
    }
    codeEl.innerHTML = highlighted;
    codeEl.classList.add("hljs");
    // 同步更新语言标签为解析后的注册名，便于 CSS 按语言定制样式。
    if (rawLang && rawLang !== lang) {
      codeEl.classList.remove(`language-${rawLang}`);
      codeEl.classList.add(`language-${lang}`);
    }
  });
  return doc.body.innerHTML;
}

/**
 * Sanitizes HTML to prevent XSS using DOMPurify.
 */
export function sanitizeHtml(html: string): string {
  return DOMPurify.sanitize(html, {
    ALLOWED_TAGS: [
      "h1", "h2", "h3", "h4", "h5", "h6",
      "p", "br", "hr", "blockquote", "pre", "code",
      "ul", "ol", "li", "dl", "dt", "dd",
      "table", "thead", "tbody", "tr", "th", "td",
      "a", "strong", "em", "del", "ins", "sub", "sup",
      "span", "div", "img", "details", "summary",
    ],
    ALLOWED_ATTR: ["href", "title", "class", "id", "src", "alt", "target", "rel"],
    ALLOW_DATA_ATTR: false,
  });
}

/**
 * Renders markdown to sanitized HTML with syntax-highlighted code blocks.
 * highlight.js is applied to fenced code blocks before sanitization; the
 * `<span class="hljs-*">` tags it produces survive DOMPurify because `span`
 * and `class` are in the allow-list.
 */
export function renderMarkdown(md: string): string {
  if (!md) return "";
  const rawHtml = marked.parse(md, { async: false }) as string;
  const highlighted = highlightCodeBlocks(rawHtml);
  return sanitizeHtml(highlighted);
}

/**
 * Renders markdown to sanitized HTML, then wraps each `<pre>` block in a
 * `.code-block-wrap` container with an "Apply" button (`code-block-apply-btn`).
 *
 * The button carries a `data-code-index` attribute matching the order of
 * appearance (0-based), so consumers can map clicks back to source content if
 * needed. The code itself can also be re-extracted from the `<pre>`'s
 * `textContent` at click time.
 *
 * Safe because it post-processes the already-sanitized HTML using DOMParser;
 * no raw user input is injected outside of `<pre>`.
 */
export function renderMarkdownWithApplyButtons(md: string): string {
  if (!md) return "";
  const sanitized = renderMarkdown(md);
  const parser = new DOMParser();
  const doc = parser.parseFromString(sanitized, "text/html");
  const pres = doc.querySelectorAll("pre");
  pres.forEach((pre, idx) => {
    const wrap = doc.createElement("div");
    wrap.className = "code-block-wrap";
    const btn = doc.createElement("button");
    btn.className = "code-block-apply-btn";
    btn.type = "button";
    btn.setAttribute("aria-label", translate("markdown.applyButtonAria"));
    btn.setAttribute("title", translate("markdown.applyButtonTitle"));
    btn.setAttribute("data-code-index", String(idx));
    btn.textContent = translate("markdown.applyButton");
    pre.parentNode?.insertBefore(wrap, pre);
    wrap.appendChild(pre);
    wrap.appendChild(btn);
  });
  return doc.body.innerHTML;
}
