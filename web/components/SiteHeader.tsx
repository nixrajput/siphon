"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { REPO_URL, SITE_NAME } from "@/lib/site";
import { useRepoStats } from "@/components/useGitHub";
import { ExtLink } from "@/components/ExtLink";

// Nav items. `match` decides the active state: docs is active for any /docs/*
// route; the in-page section links use root-relative hashes (/#id) so they work
// from any route (Next navigates home, then scrolls). GitHub is external.
const NAV: { label: string; href: string; match: (p: string) => boolean; external?: boolean }[] = [
  { label: "Features", href: "/#features", match: () => false },
  { label: "Install", href: "/#install", match: () => false },
  { label: "Support", href: "/#support", match: () => false },
  { label: "Docs", href: "/docs", match: (p) => p.startsWith("/docs") },
  { label: "GitHub", href: REPO_URL, match: () => false, external: true },
];

export function SiteHeader() {
  const pathname = usePathname() ?? "/";
  const { version } = useRepoStats();
  const [open, setOpen] = useState(false);

  return (
    <header className="sticky top-0 z-20 border-b border-(--line) bg-(--ink)/85 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <Link
          href="/"
          className="font-mono text-lg font-bold tracking-tight text-(--paper) no-underline hover:no-underline"
        >
          <span className="text-(--flow)">~/</span>
          {SITE_NAME}
        </Link>

        {/* Desktop nav */}
        <nav className="hidden items-center gap-6 text-sm md:flex">
          {NAV.map((item) =>
            item.external ? (
              <ExtLink
                key={item.label}
                href={item.href}
                className="navlink no-underline hover:no-underline"
              >
                {item.label}
              </ExtLink>
            ) : (
              <Link
                key={item.label}
                href={item.href}
                data-active={item.match(pathname)}
                className="navlink no-underline hover:no-underline"
              >
                {item.label}
              </Link>
            ),
          )}
          <VersionBadge version={version} />
        </nav>

        {/* Mobile toggle */}
        <button
          type="button"
          className="md:hidden"
          aria-label={open ? "Close menu" : "Open menu"}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="block space-y-1.5">
            <span
              className={`block h-0.5 w-6 bg-(--paper) transition-transform ${open ? "translate-y-2 rotate-45" : ""}`}
            />
            <span
              className={`block h-0.5 w-6 bg-(--paper) transition-opacity ${open ? "opacity-0" : ""}`}
            />
            <span
              className={`block h-0.5 w-6 bg-(--paper) transition-transform ${open ? "-translate-y-2 -rotate-45" : ""}`}
            />
          </span>
        </button>
      </div>

      {/* Mobile menu */}
      {open && (
        <nav className="border-t border-(--line) px-6 py-4 md:hidden">
          <ul className="space-y-3 text-sm">
            {NAV.map((item) => (
              <li key={item.label}>
                {item.external ? (
                  <ExtLink
                    href={item.href}
                    className="block text-(--muted) no-underline hover:text-(--paper)"
                    onClick={() => setOpen(false)}
                  >
                    {item.label}
                  </ExtLink>
                ) : (
                  <Link
                    href={item.href}
                    data-active={item.match(pathname)}
                    className="block text-(--muted) no-underline hover:text-(--paper) data-[active=true]:text-(--paper)"
                    onClick={() => setOpen(false)}
                  >
                    {item.label}
                  </Link>
                )}
              </li>
            ))}
            <li className="pt-1">
              <VersionBadge version={version} />
            </li>
          </ul>
        </nav>
      )}
    </header>
  );
}

// Live release badge with a pulsing status dot. Shows neutral "unreleased" copy
// until a real release tag is published (rather than claiming a version that
// doesn't exist yet), then shows the actual tag — never a broken/empty state.
function VersionBadge({ version }: { version: string | null }) {
  return (
    <ExtLink
      href={`${REPO_URL}/releases`}
      className="inline-flex items-center gap-2 rounded-full border border-(--line) bg-(--ink-2) px-3 py-1 font-mono text-xs text-(--muted) no-underline transition-colors hover:border-(--flow) hover:text-(--paper) hover:no-underline"
    >
      <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-(--flow)" aria-hidden />
      {version ?? "unreleased"}
    </ExtLink>
  );
}
