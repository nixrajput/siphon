"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";

// A sidebar link that highlights when its route is active. `exact` is for the
// Overview link (/docs), which must NOT light up on /docs/<slug> pages — only
// on the index itself. The active state gets a left flow-rail + raised bg, so
// the current page reads at a glance in a long list.
export function DocsNavLink({
  href,
  exact = false,
  children,
}: {
  href: string;
  exact?: boolean;
  children: React.ReactNode;
}) {
  const pathname = usePathname() ?? "";
  const active = exact ? pathname === href : pathname === href || pathname.startsWith(`${href}/`);

  return (
    <Link
      href={href}
      aria-current={active ? "page" : undefined}
      className={`block rounded px-2 py-1 text-sm no-underline transition-colors hover:bg-[var(--ink-2)] hover:text-[var(--paper)] hover:no-underline ${
        active
          ? "border-l-2 border-[var(--flow)] bg-[var(--ink-2)] pl-2.5 font-medium text-[var(--paper)]"
          : "text-[var(--muted)]"
      }`}
    >
      {children}
    </Link>
  );
}
