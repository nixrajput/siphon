import Link from "next/link";
import { SiteHeader } from "@/components/SiteHeader";
import { Terminal } from "@/components/Terminal";
import { InstallCommand } from "@/components/InstallCommand";
import { Reveal } from "@/components/Reveal";

const REPO = "https://github.com/nixrajput/siphon";

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

export default function Home() {
  return (
    <>
      <SiteHeader />

      {/* Hero: headline left, the product's own transcript right. The layout
          flows left→right, mirroring `siphon src dst`. */}
      <section className="mx-auto grid max-w-6xl items-center gap-10 px-6 py-20 lg:grid-cols-2 lg:py-28">
        <div>
          <p className="eyebrow rise mb-5">backup · restore · sync · cdc</p>
          <h1 className="rise text-5xl sm:text-6xl" style={{ "--rise-delay": "80ms" } as React.CSSProperties}>
            Sync any database,
            <br />
            <span className="bg-gradient-to-r from-[var(--flow)] to-[var(--flow-2)] bg-clip-text text-transparent">
              anywhere.
            </span>
          </h1>
          <p className="rise mt-6 max-w-md text-lg leading-relaxed text-[#c4d0e0]" style={{ "--rise-delay": "160ms" } as React.CSSProperties}>
            One binary that turns the painful sprawl of{" "}
            <code className="mono text-sm text-[var(--flow)]">pg_dump → pg_restore</code>{" "}
            shell scripts into a guided, observable workflow — across PostgreSQL,
            MySQL, and MariaDB.
          </p>
          <div className="rise mt-8 flex flex-wrap gap-3" style={{ "--rise-delay": "240ms" } as React.CSSProperties}>
            <Link
              href="/docs"
              className="rounded-lg bg-[var(--flow)] px-5 py-3 font-medium text-[var(--ink)] no-underline transition-opacity hover:opacity-90 hover:no-underline"
            >
              Read the docs
            </Link>
            <Link
              href="#install"
              className="rounded-lg border border-[var(--line)] px-5 py-3 font-medium text-[var(--paper)] no-underline hover:border-[var(--flow)] hover:no-underline"
            >
              Install
            </Link>
          </div>
        </div>
        <div className="rise" style={{ "--rise-delay": "320ms" } as React.CSSProperties}>
          <Terminal />
        </div>
      </section>

      {/* Feature stream: the signature flow line runs down the left margin,
          connecting capabilities like data moving through a siphon. */}
      <section className="mx-auto max-w-6xl px-6 py-12">
        <p className="eyebrow mb-10">what it does</p>
        <div className="flex gap-8">
          <div className="flowline hidden shrink-0 sm:block" aria-hidden />
          <div className="grid gap-px overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--line)] sm:grid-cols-2">
            {FEATURES.map((f, i) => (
              <Reveal key={f.label} delay={(i % 2) * 90} className="bg-[var(--ink)]">
                <article className="p-6">
                  <p className="eyebrow mb-3">{f.label}</p>
                  <h3 className="mb-2 text-xl">{f.title}</h3>
                  <p className="text-sm leading-relaxed text-[var(--muted)]">{f.body}</p>
                </article>
              </Reveal>
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
            <p className="mb-2 font-mono text-xs uppercase tracking-widest text-[var(--muted)]">
              Linux · macOS
            </p>
            <InstallCommand command="curl -fsSL https://siphon.dev/install.sh | sh" />
          </div>
          <div className="grid gap-6 sm:grid-cols-2">
            <div>
              <p className="mb-2 font-mono text-xs uppercase tracking-widest text-[var(--muted)]">
                Homebrew
              </p>
              <InstallCommand command="brew install nixrajput/siphon/siphon" />
            </div>
            <div>
              <p className="mb-2 font-mono text-xs uppercase tracking-widest text-[var(--muted)]">
                Scoop (Windows)
              </p>
              <InstallCommand command="scoop install siphon" />
            </div>
          </div>
          <p className="text-sm text-[var(--muted)]">
            Prefer source? <code className="mono text-[var(--flow)]">go install</code>{" "}
            the module, or grab a signed binary from{" "}
            <a href={`${REPO}/releases`}>Releases</a>. Every archive ships with a
            SHA-256 checksum and a cosign signature.
          </p>
        </div>
      </section>

      <footer className="border-t border-[var(--line)]">
        <div className="mx-auto flex max-w-6xl flex-col gap-2 px-6 py-8 text-sm text-[var(--muted)] sm:flex-row sm:justify-between">
          <span className="mono">~/siphon — MIT licensed</span>
          <span>
            <a href={REPO}>GitHub</a> · <Link href="/docs">Docs</Link>
          </span>
        </div>
      </footer>
    </>
  );
}
