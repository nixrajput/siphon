import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";

// The docs site is NOT a fork of the documentation — it renders the same
// Markdown that lives in the repo's docs/ directory, read at build time. A new
// or edited concept doc shows up here automatically once added to the nav map
// below, so the site can't silently drift from the code repo's docs.
const DOCS_DIR = path.join(process.cwd(), "..", "docs");

// Ordered nav: file → slug + display title. Order is the sidebar order.
const NAV: { file: string; slug: string; title: string }[] = [
  { file: "DRIVERS.md", slug: "drivers", title: "Drivers" },
  { file: "INCREMENTAL.md", slug: "incremental", title: "Incremental backup" },
  { file: "CROSS_ENGINE.md", slug: "cross-engine", title: "Cross-engine sync" },
  { file: "CDC.md", slug: "cdc", title: "CDC (continuous sync)" },
  { file: "STORAGE.md", slug: "storage", title: "Storage backends" },
  { file: "RETENTION.md", slug: "retention", title: "Retention & pruning" },
  { file: "OPS.md", slug: "ops", title: "Operational features" },
];

export type DocMeta = { slug: string; title: string };
export type Doc = DocMeta & { content: string };

export function docNav(): DocMeta[] {
  return NAV.map(({ slug, title }) => ({ slug, title }));
}

export function getDoc(slug: string): Doc | null {
  const entry = NAV.find((n) => n.slug === slug);
  if (!entry) return null;
  const raw = fs.readFileSync(path.join(DOCS_DIR, entry.file), "utf8");
  const { content } = matter(raw);
  return { slug: entry.slug, title: entry.title, content: stripInPageToc(content) };
}

// The repo docs carry their own "## Table of contents" list for GitHub viewing;
// the site provides sidebar + in-page nav instead, so drop that section to
// avoid a redundant link list at the top of every page.
function stripInPageToc(md: string): string {
  const lines = md.split("\n");
  const start = lines.findIndex((l) => /^##\s+Table of contents/i.test(l));
  if (start === -1) return md;
  // Drop from the heading until the next heading of the same-or-higher level.
  let end = start + 1;
  while (end < lines.length && !/^#{1,2}\s/.test(lines[end])) end++;
  lines.splice(start, end - start);
  return lines.join("\n").replace(/^\n+/, "");
}
