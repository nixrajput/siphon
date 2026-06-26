import fs from "node:fs";
import path from "node:path";
import matter from "gray-matter";

// The docs site is NOT a fork of the documentation — it renders the same
// Markdown that lives in the repo's docs/ directory, read at build time. A new
// or edited concept doc shows up here automatically once added to the nav map
// below, so the site can't silently drift from the code repo's docs.
const DOCS_DIR = path.join(process.cwd(), "..", "docs");

// A doc's group, used by the Overview page to cluster guides. Sidebar order is
// still the array order below.
export type DocGroup = "Start here" | "Moving data" | "Operating";

// Ordered nav: file → slug + display title + a one-line blurb (for the Overview
// cards) + its group. The blurb is site copy, not pulled from the Markdown, so
// the Overview reads as a curated index rather than a dump of first lines.
const NAV: { file: string; slug: string; title: string; blurb: string; group: DocGroup }[] = [
  {
    file: "GETTING_STARTED.md",
    slug: "getting-started",
    title: "Getting started",
    blurb: "Install siphon and run your first backup, restore, and sync in a few minutes.",
    group: "Start here",
  },
  {
    file: "CONFIGURATION.md",
    slug: "configuration",
    title: "Configuration",
    blurb: "Connection profiles, config file layout, env vars, and per-profile overrides.",
    group: "Start here",
  },
  {
    file: "DRIVERS.md",
    slug: "drivers",
    title: "Drivers",
    blurb: "How siphon talks to PostgreSQL, MySQL, and MariaDB, and what each driver supports.",
    group: "Start here",
  },
  {
    file: "INCREMENTAL.md",
    slug: "incremental",
    title: "Incremental backup",
    blurb: "Capture only what changed since a base, then replay the base→incremental chain.",
    group: "Moving data",
  },
  {
    file: "CROSS_ENGINE.md",
    slug: "cross-engine",
    title: "Cross-engine sync",
    blurb: "Move schema and data between different engines through a canonical model.",
    group: "Moving data",
  },
  {
    file: "CDC.md",
    slug: "cdc",
    title: "CDC (continuous sync)",
    blurb: "Tail the source's change stream and apply it to a target, with resumable state.",
    group: "Moving data",
  },
  {
    file: "STORAGE.md",
    slug: "storage",
    title: "Storage backends",
    blurb: "Keep dumps locally or in S3 with a key-addressed, pluggable storage seam.",
    group: "Operating",
  },
  {
    file: "RETENTION.md",
    slug: "retention",
    title: "Retention & pruning",
    blurb: "Prune whole backup chains by keep-last, max-age, or GFS without orphaning.",
    group: "Operating",
  },
  {
    file: "OPS.md",
    slug: "ops",
    title: "Operational features",
    blurb: "Audit logging, 2FA-gated destructive ops, telemetry, schedules, and tunnels.",
    group: "Operating",
  },
];

export type DocMeta = { slug: string; title: string; blurb: string; group: DocGroup };
export type Doc = { slug: string; title: string; content: string };

export function docNav(): DocMeta[] {
  return NAV.map(({ slug, title, blurb, group }) => ({ slug, title, blurb, group }));
}

// The group order the Overview renders sections in.
export const DOC_GROUPS: DocGroup[] = ["Start here", "Moving data", "Operating"];

// resolveDocHref maps an in-repo Markdown link to its site route. The docs
// cross-link each other by filename (e.g. "CONFIGURATION.md", "OPS.md#secret-
// backends", "docs/STORAGE.md"); react-markdown would render those verbatim as
// broken /docs/CONFIGURATION.md URLs. This rewrites a known doc file to
// /docs/<slug> (preserving any #hash), and returns null for anything else (http
// links, in-page anchors) so the caller leaves them untouched.
export function resolveDocHref(href: string): string | null {
  if (!href || /^[a-z]+:|^#|^\//i.test(href)) return null; // external, anchor, or absolute
  const [pathPart, hash] = href.split("#");
  const base = pathPart.replace(/^\.?\/?(docs\/)?/, "").toUpperCase();
  const entry = NAV.find((n) => n.file.toUpperCase() === base);
  if (!entry) return null;
  return `/docs/${entry.slug}${hash ? `#${hash}` : ""}`;
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
