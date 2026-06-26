import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import rehypeHighlight from "rehype-highlight";
import { resolveDocHref } from "@/lib/docs";

// Rewrite in-repo .md links to their /docs/<slug> routes so the docs'
// cross-references resolve on the site instead of 404-ing as /docs/FILE.md.
// Non-doc links (http, in-page anchors) pass through unchanged.
const components: Components = {
  a({ href = "", children, ...props }) {
    return (
      <a href={resolveDocHref(href) ?? href} {...props}>
        {children}
      </a>
    );
  },
};

// Renders repo Markdown (GFM tables, fenced code) into the docs pages. Slugged
// headings give stable anchor links; highlight.js classes style code blocks.
export function Markdown({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSlug, rehypeHighlight]}
      components={components}
    >
      {content}
    </ReactMarkdown>
  );
}
