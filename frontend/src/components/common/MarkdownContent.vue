<script lang="ts">
// MED-03: MarkdownContent is the single safe boundary for rendering
// pre-sanitized HTML. It uses a render function (h) with innerHTML instead
// of the v-html directive, so the eslint vue/no-v-html rule stays effective
// across the rest of the app.
//
// Callers MUST pass HTML that has already been run through DOMPurify
// (renderMarkdown / renderMarkdownWithApplyButtons). Fallthrough attrs
// (class, style, @click) are merged onto the rendered div so callers can
// style and interact with the content exactly as before.
import { h, type FunctionalComponent } from "vue";

interface MarkdownContentProps {
  html: string;
}

const MarkdownContent: FunctionalComponent<MarkdownContentProps> = (
  props,
  { attrs, slots },
) => {
  // When no html is provided, fall back to the default slot so callers can
  // use this component as a plain styled container too.
  if (!props.html) {
    return h("div", attrs, slots.default?.());
  }
  return h("div", { ...attrs, innerHTML: props.html });
};

MarkdownContent.props = ["html"];
MarkdownContent.inheritAttrs = true;

export default MarkdownContent;
</script>
