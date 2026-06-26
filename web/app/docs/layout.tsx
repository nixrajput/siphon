import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { DocsNavLink } from "@/components/DocsNavLink";
import { docNav } from "@/lib/docs";

// Shared docs shell: site header + a sticky sidebar nav over the concept docs.
export default function DocsLayout({ children }: { children: React.ReactNode }) {
  const nav = docNav();
  return (
    <>
      <SiteHeader />
      <div className="mx-auto flex min-h-[60vh] max-w-6xl gap-10 px-6 py-12">
        <aside className="hidden w-56 shrink-0 lg:block">
          <nav className="sticky top-24">
            <p className="eyebrow mb-4">documentation</p>
            <ul className="space-y-1">
              <li>
                <DocsNavLink href="/docs" exact>
                  Overview
                </DocsNavLink>
              </li>
              {nav.map((d) => (
                <li key={d.slug}>
                  <DocsNavLink href={`/docs/${d.slug}`}>{d.title}</DocsNavLink>
                </li>
              ))}
            </ul>
          </nav>
        </aside>
        <div className="min-w-0 flex-1">{children}</div>
      </div>
      <SiteFooter />
    </>
  );
}
