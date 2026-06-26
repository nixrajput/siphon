import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { SiteFooter } from "@/components/SiteFooter";
import { Terminal } from "@/components/Terminal";
import { InstallCommand } from "@/components/InstallCommand";
import { HeroInstall } from "@/components/HeroInstall";
import { DeveloperSection } from "@/components/DeveloperSection";
import { SupportSection } from "@/components/SupportSection";
import { Reveal } from "@/components/Reveal";
import { ExtLink } from "@/components/ExtLink";
import { REPO_URL, INSTALL_CMD, ENGINES } from "@/lib/site";

// Features are siphon's real capabilities, named by what the engineer does with
// them — not by how they're built. Each is one true claim, no filler.
const FEATURES: { label: string; title: string; body: string }[] = [
  {
    label: "transfer",
    title: "Stream prod to staging, no temp file",
    body: "siphon sync pipes a backup straight into a restore through a bounded buffer. A failed dump never lands as a clean restore — backpressure and failures propagate end to end.",
  },
  {
    label: "incremental",
    title: "Capture only what changed",
    body: "backup --incremental records a bounded change set since the base via Postgres logical decoding or MySQL/MariaDB binlog. restore replays the base→incremental chain in order.",
  },
  {
    label: "cross-engine",
    title: "Postgres → MySQL, and back",
    body: "Typed schema introspection pivots through a canonical model, so data and table structure move between engines with per-engine quoting and bound parameters.",
  },
  {
    label: "cdc",
    title: "Follow changes continuously",
    body: "siphon cdc tails the source's change stream and applies each change to the target — same-engine or cross-engine — with a snapshot→stream handoff and resumable state.",
  },
  {
    label: "integrity",
    title: "Every dump is checksummed",
    body: "SHA-256 over the envelope and body, recorded in a sidecar. siphon verify re-hashes and fails with a distinct exit code, so CI catches corruption or tampering.",
  },
  {
    label: "ops",
    title: "Cloud storage, retention, audit, 2FA",
    body: "Keep dumps in S3, prune whole chains by keep-last / max-age / GFS, log destructive ops, and gate them behind a typed confirmation or a TOTP code per profile.",
  },
];

// The pipeline IS a real sequence (source → engine → target), so numbered
// stages are honest here, not decoration.
const FLOW: { n: string; title: string; body: string; cmd: string }[] = [
  {
    n: "01",
    title: "Point at a source",
    body: "A connection string or a saved profile. siphon introspects the schema and picks the right driver — Postgres, MySQL, or MariaDB.",
    cmd: "siphon backup prod",
  },
  {
    n: "02",
    title: "siphon moves it",
    body: "Dump, stream, or tail. Everything flows through a bounded buffer with backpressure, checksums, and a live progress view.",
    cmd: "siphon sync prod staging",
  },
  {
    n: "03",
    title: "Land it anywhere",
    body: "Restore to another database, a different engine, or object storage. The base→incremental chain replays in order, verified end to end.",
    cmd: "siphon cdc prod replica",
  },
];

export default function Home() {
  return (
    <>
      <SiteHeader />

      {/* Hero: headline + live action block left, the product's own transcript
          right. A drifting conduit grid sits behind, evoking fluid in pipes. */}
      <section className="relative overflow-hidden">
        <div className="flowgrid pointer-events-none absolute inset-0 -z-10" aria-hidden />
        <div className="mx-auto grid max-w-6xl items-center gap-10 px-6 py-20 lg:grid-cols-2 lg:py-28">
          <div>
            <p className="eyebrow rise mb-5">backup · restore · sync · cdc</p>
            <h1
              className="rise text-[2.6rem] leading-[1.05] text-balance sm:text-6xl"
              style={{ "--rise-delay": "80ms" } as React.CSSProperties}
            >
              Sync any database,
              <br />
              <span className="bg-linear-to-r from-(--flow) to-(--flow-2) bg-clip-text text-transparent">
                anywhere.
              </span>
            </h1>
            <p
              className="rise mt-6 max-w-md text-lg leading-relaxed text-[#c4d0e0]"
              style={{ "--rise-delay": "160ms" } as React.CSSProperties}
            >
              One binary that turns the painful sprawl of{" "}
              <code className="mono text-sm text-(--flow)">pg_dump → pg_restore</code> shell scripts
              into a guided, observable workflow — across PostgreSQL, MySQL, and MariaDB.
            </p>
            <div className="rise mt-8" style={{ "--rise-delay": "240ms" } as React.CSSProperties}>
              <HeroInstall />
            </div>
            <div
              className="rise mt-5 flex flex-wrap gap-3"
              style={{ "--rise-delay": "320ms" } as React.CSSProperties}
            >
              <Link
                href="/docs"
                className="rounded-lg bg-(--flow) px-5 py-3 font-medium text-(--ink) no-underline transition-opacity hover:no-underline hover:opacity-90"
              >
                Read the docs
              </Link>
              <ExtLink
                href={REPO_URL}
                className="rounded-lg border border-(--line) px-5 py-3 font-medium text-(--paper) no-underline transition-colors hover:border-(--flow) hover:no-underline"
              >
                View source ↗
              </ExtLink>
            </div>
          </div>
          <div className="rise" style={{ "--rise-delay": "400ms" } as React.CSSProperties}>
            <Terminal />
          </div>
        </div>
      </section>

      {/* How it flows: the real source → engine → target sequence. */}
      <section className="mx-auto max-w-6xl px-6 py-12">
        <p className="eyebrow mb-3">how it flows</p>
        <h2 className="mb-10 max-w-2xl text-3xl">Three verbs, one conduit</h2>
        <div className="grid gap-px overflow-hidden rounded-xl border border-(--line) bg-(--line) md:grid-cols-3">
          {FLOW.map((s, i) => (
            <Reveal key={s.n} delay={i * 90} className="bg-(--ink)">
              <article className="flex h-full flex-col p-6">
                <span className="mono text-3xl font-bold text-(--flow)">{s.n}</span>
                <h3 className="mt-4 text-xl">{s.title}</h3>
                <p className="mt-2 flex-1 text-sm leading-relaxed text-(--muted)">{s.body}</p>
                <code className="mt-4 block rounded-md border border-(--line) bg-[#0a111e] px-3 py-2 font-mono text-xs text-(--paper)">
                  <span className="text-(--amber)">$ </span>
                  {s.cmd}
                </code>
              </article>
            </Reveal>
          ))}
        </div>
      </section>

      {/* Feature stream: the signature flow line runs down the left margin,
          connecting capabilities like data moving through a siphon. */}
      <section className="mx-auto max-w-6xl px-6 py-12">
        <p className="eyebrow mb-10">what it does</p>
        <div className="flex gap-8">
          <div className="flowline hidden shrink-0 sm:block" aria-hidden />
          <div className="grid gap-px overflow-hidden rounded-xl border border-(--line) bg-(--line) sm:grid-cols-2">
            {FEATURES.map((f, i) => (
              <Reveal key={f.label} delay={(i % 2) * 90} className="bg-(--ink)">
                <article className="p-6">
                  <p className="eyebrow mb-3">{f.label}</p>
                  <h3 className="mb-2 text-xl">{f.title}</h3>
                  <p className="text-sm leading-relaxed text-(--muted)">{f.body}</p>
                </article>
              </Reveal>
            ))}
          </div>
        </div>
      </section>

      {/* Engines strip: the three databases siphon speaks, stated once. */}
      <section className="mx-auto max-w-6xl px-6 py-12">
        <div className="flex flex-col items-start justify-between gap-6 rounded-xl border border-(--line) bg-(--ink-2) p-8 sm:flex-row sm:items-center">
          <div>
            <p className="eyebrow mb-2">speaks natively</p>
            <p className="max-w-md text-sm leading-relaxed text-(--muted)">
              First-class drivers for each engine — same commands, engine-aware quoting, types, and
              change capture under the hood.
            </p>
          </div>
          <div className="flex flex-wrap gap-3">
            {ENGINES.map((e) => (
              <span
                key={e}
                className="rounded-full border border-(--line) bg-(--ink) px-4 py-2 font-mono text-sm text-(--flow)"
              >
                {e}
              </span>
            ))}
          </div>
        </div>
      </section>

      {/* Install: the one amber moment. Three ways, the curl|sh widget leads. */}
      <section id="install" className="mx-auto max-w-3xl scroll-mt-24 px-6 py-20">
        <p className="eyebrow mb-4">install</p>
        <h2 className="mb-8 text-3xl">Up and running in one line</h2>
        <div className="space-y-6">
          <div>
            <p className="mb-2 font-mono text-xs tracking-widest text-(--muted) uppercase">
              Linux · macOS
            </p>
            <InstallCommand command={INSTALL_CMD} />
          </div>
          <div className="grid gap-6 sm:grid-cols-2">
            <div>
              <p className="mb-2 font-mono text-xs tracking-widest text-(--muted) uppercase">
                Homebrew
              </p>
              <InstallCommand command="brew install nixrajput/siphon/siphon" />
            </div>
            <div>
              <p className="mb-2 font-mono text-xs tracking-widest text-(--muted) uppercase">
                Scoop (Windows)
              </p>
              <InstallCommand command="scoop install siphon" />
            </div>
          </div>
          <p className="text-sm text-(--muted)">
            Prefer source? <code className="mono text-(--flow)">go install</code> the module, or
            grab a prebuilt binary from <ExtLink href={`${REPO_URL}/releases`}>Releases</ExtLink>.
            Every archive has a SHA-256 checksum, and the checksum file is cosign-signed.
          </p>
        </div>
      </section>

      <DeveloperSection />

      <SupportSection />

      <SiteFooter />
    </>
  );
}
