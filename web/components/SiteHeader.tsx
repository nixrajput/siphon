import Link from "next/link";

const REPO = "https://github.com/nixrajput/siphon";

// Wordmark sets the monospace-as-identity tone: the name reads like a command.
export function SiteHeader() {
  return (
    <header className="sticky top-0 z-10 border-b border-[var(--line)] bg-[var(--ink)]/85 backdrop-blur">
      <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
        <Link href="/" className="font-mono text-lg font-bold tracking-tight text-[var(--paper)] no-underline hover:no-underline">
          <span className="text-[var(--flow)]">~/</span>siphon
        </Link>
        <nav className="flex items-center gap-6 text-sm">
          <Link href="/docs" className="text-[var(--muted)] no-underline hover:text-[var(--paper)]">
            Docs
          </Link>
          <Link href="/#install" className="text-[var(--muted)] no-underline hover:text-[var(--paper)]">
            Install
          </Link>
          <a href={REPO} className="text-[var(--muted)] no-underline hover:text-[var(--paper)]">
            GitHub
          </a>
        </nav>
      </div>
    </header>
  );
}
