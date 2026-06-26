import { FUNDING } from "@/lib/site";
import { ExtLink } from "@/components/ExtLink";

// Support / funding section. Mirrors the GitHub profile's "Support My Work":
// a short ask + the funding links. GitHub Sponsors leads as the primary action
// (amber, the page's single call-to-action color); the rest are secondary.
export function SupportSection() {
  const [primary, ...rest] = FUNDING;

  return (
    <section id="support" className="mx-auto max-w-3xl scroll-mt-24 px-6 py-20 text-center">
      <p className="eyebrow mb-4">support</p>
      <h2 className="mb-4 text-3xl">Free and open source</h2>
      <p className="mx-auto mb-8 max-w-md leading-relaxed text-(--muted)">
        siphon is free to use. If it saves you time, you can support its continued development —
        every bit helps and is genuinely appreciated. ❤️
      </p>
      <div className="flex flex-wrap items-center justify-center gap-3">
        <ExtLink
          href={primary.url}
          className="rounded-lg bg-(--amber) px-5 py-3 font-medium text-(--ink) no-underline transition-opacity hover:no-underline hover:opacity-90"
        >
          {primary.label} ↗
        </ExtLink>
        {rest.map((f) => (
          <ExtLink
            key={f.label}
            href={f.url}
            className="rounded-lg border border-(--line) px-5 py-3 font-medium text-(--paper) no-underline transition-colors hover:border-(--amber) hover:no-underline"
          >
            {f.label} ↗
          </ExtLink>
        ))}
      </div>
    </section>
  );
}
