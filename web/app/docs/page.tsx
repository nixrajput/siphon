import Link from "next/link";
import { docNav } from "@/lib/docs";

export const metadata = { title: "Documentation — siphon" };

export default function DocsIndex() {
  const nav = docNav();
  return (
    <main>
      <p className="eyebrow mb-3">documentation</p>
      <h1 className="mb-3 text-4xl">Concepts &amp; guides</h1>
      <p className="mb-10 max-w-xl text-[var(--muted)]">
        These pages render the same Markdown that ships in the repository, so
        they always match the version you installed.
      </p>
      <div className="grid gap-px overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--line)] sm:grid-cols-2">
        {nav.map((d) => (
          <Link
            key={d.slug}
            href={`/docs/${d.slug}`}
            className="block bg-[var(--ink)] p-5 no-underline transition-colors hover:bg-[var(--ink-2)] hover:no-underline"
          >
            <span className="text-lg font-semibold text-[var(--paper)]">{d.title}</span>
          </Link>
        ))}
      </div>
    </main>
  );
}
