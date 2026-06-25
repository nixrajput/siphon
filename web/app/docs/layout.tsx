import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { docNav } from "@/lib/docs";

// Shared docs shell: site header + a sticky sidebar nav over the concept docs.
export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const nav = docNav();
  return (
    <>
      <SiteHeader />
      <div className="mx-auto flex max-w-6xl gap-10 px-6 py-12">
        <aside className="hidden w-56 shrink-0 lg:block">
          <nav className="sticky top-24">
            <p className="eyebrow mb-4">documentation</p>
            <ul className="space-y-1">
              <li>
                <Link
                  href="/docs"
                  className="block rounded px-2 py-1 text-sm text-[var(--muted)] no-underline hover:bg-[var(--ink-2)] hover:text-[var(--paper)] hover:no-underline"
                >
                  Overview
                </Link>
              </li>
              {nav.map((d) => (
                <li key={d.slug}>
                  <Link
                    href={`/docs/${d.slug}`}
                    className="block rounded px-2 py-1 text-sm text-[var(--muted)] no-underline hover:bg-[var(--ink-2)] hover:text-[var(--paper)] hover:no-underline"
                  >
                    {d.title}
                  </Link>
                </li>
              ))}
            </ul>
          </nav>
        </aside>
        <div className="min-w-0 flex-1">{children}</div>
      </div>
    </>
  );
}
