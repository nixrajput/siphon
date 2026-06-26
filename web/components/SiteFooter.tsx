import Link from "next/link";
import { REPO_URL, DEVELOPER, SITE_NAME } from "@/lib/site";
import { ExtLink } from "@/components/ExtLink";

// The closing credit. Three columns on wide screens — the project, the docs,
// and the developer — collapsing to a stack on mobile. The amber wordmark
// rhymes with the install moment; everything else stays in the muted register
// so the footer reads as a quiet sign-off, not a second navigation.

export function SiteFooter() {
  // Server component: the year is resolved at build/request time (re-baked on
  // each redeploy), so the copyright never goes stale.
  const year = new Date().getFullYear();
  return (
    <footer className="mt-24 border-t border-(--line)">
      <div className="mx-auto grid max-w-6xl gap-10 px-6 py-12 sm:grid-cols-2 lg:grid-cols-4">
        <div className="lg:col-span-2">
          <Link
            href="/"
            className="font-mono text-lg font-bold tracking-tight text-(--paper) no-underline hover:no-underline"
          >
            <span className="text-(--flow)">~/</span>
            {SITE_NAME}
          </Link>
          <p className="mt-3 max-w-xs text-sm leading-relaxed text-(--muted)">
            One binary for database backup, restore, sync, and CDC across PostgreSQL, MySQL, and
            MariaDB.
          </p>
        </div>

        <div>
          <p className="eyebrow mb-3">Project</p>
          <ul className="space-y-2 text-sm">
            <li>
              <Link href="/docs" className="text-(--muted) no-underline hover:text-(--paper)">
                Documentation
              </Link>
            </li>
            <li>
              <ExtLink href={REPO_URL} className="text-(--muted) no-underline hover:text-(--paper)">
                Source on GitHub
              </ExtLink>
            </li>
            <li>
              <ExtLink
                href={`${REPO_URL}/releases`}
                className="text-(--muted) no-underline hover:text-(--paper)"
              >
                Releases
              </ExtLink>
            </li>
            <li>
              <Link href="/#install" className="text-(--muted) no-underline hover:text-(--paper)">
                Install
              </Link>
            </li>
          </ul>
        </div>

        <div>
          <p className="eyebrow mb-3">Developer</p>
          <ul className="space-y-2 text-sm">
            <li>
              <ExtLink
                href={DEVELOPER.portfolio}
                className="text-(--muted) no-underline hover:text-(--paper)"
              >
                Portfolio
              </ExtLink>
            </li>
            <li>
              <ExtLink
                href={DEVELOPER.github}
                className="text-(--muted) no-underline hover:text-(--paper)"
              >
                GitHub (@{DEVELOPER.handle})
              </ExtLink>
            </li>
          </ul>
        </div>
      </div>

      <div className="border-t border-(--line)">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 px-6 py-6 text-xs text-(--muted) sm:flex-row sm:items-center sm:justify-between">
          <span className="mono">
            © {year} {DEVELOPER.name} · MIT licensed
          </span>
          <span className="mono">
            <span className="text-(--flow)">~/</span>
            {SITE_NAME}
          </span>
        </div>
      </div>
    </footer>
  );
}
