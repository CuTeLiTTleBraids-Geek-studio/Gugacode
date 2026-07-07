import { describe, it, expect, vi } from "vitest";

// N-148: markdown.ts now imports translate from @/lib/i18n, which transitively
// imports @/stores/app → @/lib/monaco-themes (fails under jsdom). Mock the
// heavy modules so the test can import the markdown module cleanly.
vi.mock("@/lib/monaco-themes", () => ({
  accentThemes: {},
  applyMonacoTheme: vi.fn(),
  applyMonacoThemeForMode: vi.fn(),
  registerAllThemes: vi.fn(),
}));

vi.mock("@/api/services", () => ({
  settingsService: {
    loadSettings: vi.fn().mockResolvedValue({}),
    saveSettings: vi.fn().mockResolvedValue(undefined),
  },
}));

vi.mock("@wailsio/runtime", () => ({
  Events: { On: vi.fn() },
}));

import { renderMarkdown, renderMarkdownWithApplyButtons, sanitizeHtml } from "./markdown";

describe("renderMarkdown", () => {
  it("renders plain text as a paragraph", () => {
    const html = renderMarkdown("hello world");
    expect(html).toContain("hello world");
    expect(html).toContain("<p>");
  });

  it("renders fenced code blocks with language class", () => {
    const md = "```js\nconst x = 1;\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("<code");
    // "js" alias is normalized to "javascript" by resolveLanguage()
    expect(html).toContain("language-javascript");
    expect(html).toContain("hljs");
    // After highlight.js, "const" is wrapped in a keyword span
    expect(html).toContain("hljs-keyword");
    // Code content is preserved (split across spans but still present)
    expect(html).toContain("const");
    expect(html).toContain("x");
  });

  it("highlights TypeScript with ts alias", () => {
    const md = "```ts\ninterface Foo { bar: string }\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    // "interface" should be recognized as a TS keyword
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("interface");
    // alias ts → typescript: class should be normalized
    expect(html).toContain("language-typescript");
  });

  it("highlights Go with go/golang alias", () => {
    const md = "```go\npackage main\nfunc main() {}\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("package");
    expect(html).toContain("func");
  });

  it("highlights Java", () => {
    const md = "```java\npublic class Hello { public static void main() {} }\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("public");
    expect(html).toContain("class");
  });

  it("highlights Python with py alias", () => {
    const md = "```py\ndef hello():\n    return 'hi'\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("def");
    expect(html).toContain("language-python");
  });

  it("highlights Rust with rs alias", () => {
    const md = "```rs\nfn main() { let x = 1; }\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("hljs-keyword");
    expect(html).toContain("fn");
    expect(html).toContain("language-rust");
  });

  it("highlights Bash with sh alias", () => {
    const md = "```sh\necho hello\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("language-bash");
  });

  it("highlights YAML with yml alias", () => {
    const md = "```yml\nkey: value\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    expect(html).toContain("language-yaml");
  });

  it("falls back to auto-detection for unknown language", () => {
    const md = "```xyzlang\nsome random text\n```";
    const html = renderMarkdown(md);
    expect(html).toContain("hljs");
    // Auto-detection may wrap parts of the text in spans; just verify the
    // code block is present and the raw text survives (possibly split).
    expect(html).toContain("some random");
    expect(html).toContain("text");
  });

  it("renders inline code", () => {
    const html = renderMarkdown("use `const` for declarations");
    expect(html).toContain("<code>const</code>");
  });

  it("renders bold text", () => {
    const html = renderMarkdown("**important**");
    expect(html).toContain("<strong>important</strong>");
  });

  it("renders bullet lists", () => {
    const md = "- one\n- two\n- three";
    const html = renderMarkdown(md);
    expect(html).toContain("<ul>");
    expect(html).toContain("<li>one</li>");
    expect(html).toContain("<li>three</li>");
  });

  it("renders links with href", () => {
    const html = renderMarkdown("[docs](https://example.com)");
    expect(html).toContain('<a href="https://example.com"');
    expect(html).toContain(">docs</a>");
  });

  it("renders headers", () => {
    expect(renderMarkdown("# Title")).toContain("<h1>");
    expect(renderMarkdown("## Sub")).toContain("<h2>");
  });

  it("escapes raw HTML in content", () => {
    const html = renderMarkdown("<script>alert(1)</script>");
    expect(html).not.toContain("<script>");
  });
});

describe("sanitizeHtml", () => {
  it("strips script tags", () => {
    const result = sanitizeHtml("<p>ok</p><script>alert(1)</script>");
    expect(result).not.toContain("<script>");
    expect(result).toContain("<p>ok</p>");
  });

  it("strips on* attributes", () => {
    const result = sanitizeHtml('<p onclick="evil()">text</p>');
    expect(result).not.toContain("onclick");
    expect(result).toContain("text");
  });

  it("allows safe tags", () => {
    const result = sanitizeHtml("<strong>bold</strong><em>italic</em>");
    expect(result).toContain("<strong>bold</strong>");
    expect(result).toContain("<em>italic</em>");
  });
});

describe("markdown XSS prevention", () => {
  it("strips script tags", () => {
    const html = renderMarkdown('<script>alert("xss")</script>hello');
    expect(html).not.toContain("<script>");
    expect(html).not.toContain("alert");
  });

  it("strips javascript: URLs", () => {
    const html = renderMarkdown('[click](javascript:alert("xss"))');
    expect(html).not.toContain("javascript:");
  });

  it("strips onerror handlers", () => {
    const html = renderMarkdown('<img src="x" onerror="alert(1)">');
    expect(html).not.toContain("onerror");
  });

  it("preserves safe markdown", () => {
    const html = renderMarkdown("**bold** and `code`");
    expect(html).toContain("<strong>bold</strong>");
    expect(html).toContain("<code>code</code>");
  });
});

describe("renderMarkdownWithApplyButtons", () => {
  it("wraps each pre block in a code-block-wrap with Apply button", () => {
    const md = "```js\nconst x = 1;\n```\n\n```go\nfmt.Println()\n```";
    const html = renderMarkdownWithApplyButtons(md);
    expect(html).toContain('class="code-block-wrap"');
    expect(html).toContain('class="code-block-apply-btn"');
    // Two buttons for two code blocks
    const buttonCount = (html.match(/code-block-apply-btn/g) || []).length;
    expect(buttonCount).toBe(2);
  });

  it("assigns sequential data-code-index to buttons", () => {
    const md = "```\none\n```\n\n```\ntwo\n```";
    const html = renderMarkdownWithApplyButtons(md);
    expect(html).toContain('data-code-index="0"');
    expect(html).toContain('data-code-index="1"');
  });

  it("places the button after the pre inside the wrap", () => {
    const md = "```js\nconst y = 2;\n```";
    const html = renderMarkdownWithApplyButtons(md);
    // The wrap should contain pre then button (button comes after pre in source order)
    const wrapStart = html.indexOf('class="code-block-wrap"');
    const prePos = html.indexOf("<pre", wrapStart);
    const btnPos = html.indexOf('class="code-block-apply-btn"', wrapStart);
    expect(prePos).toBeGreaterThan(-1);
    expect(btnPos).toBeGreaterThan(prePos);
  });

  it("returns empty string for empty input", () => {
    expect(renderMarkdownWithApplyButtons("")).toBe("");
  });

  it("preserves code content inside the pre", () => {
    const md = "```python\nprint('hello')\n```";
    const html = renderMarkdownWithApplyButtons(md);
    // After highlight.js, "print" and "hello" are in separate spans
    expect(html).toContain("print");
    expect(html).toContain("hello");
  });

  it("does not add apply button when there are no code blocks", () => {
    const html = renderMarkdownWithApplyButtons("just **bold** text");
    expect(html).not.toContain("code-block-apply-btn");
    expect(html).not.toContain("code-block-wrap");
  });

  // N-148: Apply button text must come from i18n, not be hardcoded
  it("N-148: Apply button uses i18n label, aria-label, and title", () => {
    const md = "```js\nconst x = 1;\n```";
    const html = renderMarkdownWithApplyButtons(md);
    // Default locale is "en" — verify the English i18n values appear
    expect(html).toContain(">Apply<");
    expect(html).toContain('aria-label="Apply code block to current file"');
    expect(html).toContain('title="Apply to current file"');
  });
});