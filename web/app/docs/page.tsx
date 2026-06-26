import Link from "next/link";
import { docNav, DOC_GROUPS } from "@/lib/docs";
import { InstallCommand } from "@/components/InstallCommand";
import { ExtLink } from "@/components/ExtLink";
import { INSTALL_CMD, REPO_URL } from "@/lib/site";

export const metadata = {
  title: "Documentation",
  description:
    "Concepts and guides for siphon — install, configure, and run backup, restore, sync, incremental, cross-engine, and CDC across PostgreSQL, MySQL, and MariaDB.",
  alternates: { canonical: "/docs" },
};

export default function DocsIndex() {
  const nav = docNav();

  return (
    <main>
      <p className="eyebrow mb-3">documentation</p>
      <h1 className="mb-4 text-4xl">Concepts &amp; guides</h1>
      <p className="mb-8 max-w-2xl leading-relaxed text-[var(--muted)]">
        Everything you need to run siphon in anger — from your first backup to continuous
        cross-engine replication. These pages render the same Markdown that ships in the repository,
        so they track the latest source.
      </p>

      {/* Quick start: get someone to a working install without leaving the
          Overview, then point them at the first guide. */}
      <div className="mb-12 rounded-xl border border-[var(--line)] bg-[var(--ink-2)] p-6">
        <p className="eyebrow mb-3">quick start</p>
        <p className="mb-4 max-w-xl text-sm leading-relaxed text-[var(--muted)]">
          Install the binary, then head to{" "}
          <Link href="/docs/getting-started" className="text-[var(--flow)]">
            Getting started
          </Link>{" "}
          for your first backup and sync.
        </p>
        <InstallCommand command={INSTALL_CMD} />
      </div>

      {/* Grouped guide index: each card carries a real description, not just a
          title, so the page is scannable and tells you where to go. */}
      {DOC_GROUPS.map((group) => {
        const items = nav.filter((d) => d.group === group);
        return (
          <section key={group} className="mb-10">
            <p className="eyebrow mb-4">{group}</p>
            {/* Self-bordered cards with real gap: an odd item count leaves clean
                empty space instead of a phantom filler cell (which a seam-grid
                with a colored background would show). */}
            <div className="grid gap-3 sm:grid-cols-2">
              {items.map((d) => (
                <Link
                  key={d.slug}
                  href={`/docs/${d.slug}`}
                  className="bg-[var(--ink-2)]/40 group block rounded-xl border border-[var(--line)] p-5 no-underline transition-colors hover:border-[var(--flow)] hover:bg-[var(--ink-2)] hover:no-underline"
                >
                  <span className="flex items-center gap-2 text-lg font-semibold text-[var(--paper)]">
                    {d.title}
                    <span className="text-[var(--flow)] opacity-0 transition-all group-hover:translate-x-0.5 group-hover:opacity-100">
                      →
                    </span>
                  </span>
                  <p className="mt-1.5 text-sm leading-relaxed text-[var(--muted)]">{d.blurb}</p>
                </Link>
              ))}
            </div>
          </section>
        );
      })}

      <div className="mt-12 flex flex-col gap-3 border-t border-[var(--line)] pt-8 text-sm text-[var(--muted)] sm:flex-row sm:items-center sm:justify-between">
        <span>
          Found a gap or a bug?{" "}
          <ExtLink href={`${REPO_URL}/issues`} className="text-[var(--flow)]">
            Open an issue
          </ExtLink>
          .
        </span>
        <ExtLink href={REPO_URL} className="text-[var(--flow)]">
          Browse the source ↗
        </ExtLink>
      </div>
    </main>
  );
}
