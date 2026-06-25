import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import rehypeSlug from "rehype-slug";
import rehypeHighlight from "rehype-highlight";

// Renders repo Markdown (GFM tables, fenced code) into the docs pages. Slugged
// headings give stable anchor links; highlight.js classes style code blocks.
export function Markdown({ content }: { content: string }) {
  return (
    <ReactMarkdown
      remarkPlugins={[remarkGfm]}
      rehypePlugins={[rehypeSlug, rehypeHighlight]}
    >
      {content}
    </ReactMarkdown>
  );
}
