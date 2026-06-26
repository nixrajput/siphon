"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import { useState } from "react";
import { REPO_URL, SITE_NAME } from "@/lib/site";
import { useRepoStats } from "@/components/useGitHub";
import { ExtLink } from "@/components/ExtLink";

// Nav items. `match` decides the active state: docs is active for any /docs/*
// route; the others match their exact path (or hash target on home).
const NAV: { label: string; href: string; match: (p: string) => boolean; external?: boolean }[] = [
  { label: "Docs", href: "/docs", match: (p) => p.startsWith("/docs") },
  { label: "Install", href: "/#install", match: () => false },
  { label: "GitHub", href: REPO_URL, match: () => false, external: true },
];

export function SiteHeader() {
  const pathname = usePathname() ?? "/";
  const { version } = useRepoStats();
  const [open, setOpen] = useState(false);

  return (
    <header className="bg-[var(--ink)]/85 sticky top-0 z-20 border-b border-[var(--line)] backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <Link
          href="/"
          className="font-mono text-lg font-bold tracking-tight text-[var(--paper)] no-underline hover:no-underline"
        >
          <span className="text-[var(--flow)]">~/</span>
          {SITE_NAME}
        </Link>

        {/* Desktop nav */}
        <nav className="hidden items-center gap-7 text-sm sm:flex">
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
          className="sm:hidden"
          aria-label={open ? "Close menu" : "Open menu"}
          aria-expanded={open}
          onClick={() => setOpen((v) => !v)}
        >
          <span className="block space-y-1.5">
            <span
              className={`block h-0.5 w-6 bg-[var(--paper)] transition-transform ${open ? "translate-y-2 rotate-45" : ""}`}
            />
            <span
              className={`block h-0.5 w-6 bg-[var(--paper)] transition-opacity ${open ? "opacity-0" : ""}`}
            />
            <span
              className={`block h-0.5 w-6 bg-[var(--paper)] transition-transform ${open ? "-translate-y-2 -rotate-45" : ""}`}
            />
          </span>
        </button>
      </div>

      {/* Mobile menu */}
      {open && (
        <nav className="border-t border-[var(--line)] px-6 py-4 sm:hidden">
          <ul className="space-y-3 text-sm">
            {NAV.map((item) => (
              <li key={item.label}>
                {item.external ? (
                  <ExtLink
                    href={item.href}
                    className="block text-[var(--muted)] no-underline hover:text-[var(--paper)]"
                    onClick={() => setOpen(false)}
                  >
                    {item.label}
                  </ExtLink>
                ) : (
                  <Link
                    href={item.href}
                    data-active={item.match(pathname)}
                    className="block text-[var(--muted)] no-underline hover:text-[var(--paper)] data-[active=true]:text-[var(--paper)]"
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
      className="inline-flex items-center gap-2 rounded-full border border-[var(--line)] bg-[var(--ink-2)] px-3 py-1 font-mono text-xs text-[var(--muted)] no-underline transition-colors hover:border-[var(--flow)] hover:text-[var(--paper)] hover:no-underline"
    >
      <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-[var(--flow)]" aria-hidden />
      {version ?? "unreleased"}
    </ExtLink>
  );
}
