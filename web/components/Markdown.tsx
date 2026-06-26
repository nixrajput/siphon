import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import rehypeHighlight from "rehype-highlight";
import { resolveDocHref } from "@/lib/docs";

// Rewrite in-repo .md links to their /docs/<slug> routes so the docs'
// cross-references resolve on the site instead of 404-ing as /docs/FILE.md.
// In-page anchors pass through unchanged; absolute http(s) links are external
// and open in a new tab with a safe rel.
const components: Components = {
  a({ href = "", children, ...props }) {
    const internal = resolveDocHref(href);
    const isExternal = /^https?:\/\//i.test(href);
    return (
      <a
        href={internal ?? href}
        {...(isExternal ? { target: "_blank", rel: "noopener noreferrer" } : {})}
        {...props}
      >
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
