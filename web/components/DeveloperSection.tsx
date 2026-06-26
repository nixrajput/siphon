"use client";

import { useDevStats } from "@/components/useGitHub";
import { DEVELOPER, GH_OWNER } from "@/lib/site";
import { ExtLink } from "@/components/ExtLink";
import { Skeleton } from "@/components/Skeleton";

// Labels for the three stat tiles, in render order. Used to size the loading
// skeleton so it matches the real tiles that replace it.
const STAT_LABELS = ["Followers", "Total stars", "Public repos"];

// "Built by" section: live GitHub presence for the developer. While the fetch is
// in flight, the stats + top-repos render shimmering skeletons; on success they
// fill with data; on failure they hide and the section stands as a plain credit.
export function DeveloperSection() {
  const { status, followers, publicRepos, totalStars, topRepos } = useDevStats();
  const loading = status === "loading";

  const tiles = [
    followers !== null && { label: "Followers", value: followers },
    totalStars !== null && { label: "Total stars", value: totalStars },
    publicRepos !== null && { label: "Public repos", value: publicRepos },
  ].filter(Boolean) as { label: string; value: number }[];

  return (
    <section className="mx-auto max-w-6xl px-6 py-20">
      <p className="eyebrow mb-4">built by</p>
      <div className="grid gap-10 lg:grid-cols-[1fr_1.4fr] lg:items-start">
        <div>
          <h2 className="text-3xl">
            Made by{" "}
            <ExtLink
              href={DEVELOPER.portfolio}
              className="bg-gradient-to-r from-[var(--flow)] to-[var(--flow-2)] bg-clip-text text-transparent hover:no-underline"
            >
              {DEVELOPER.name}
            </ExtLink>
          </h2>
          <p className="mt-4 max-w-md leading-relaxed text-[var(--muted)]">
            Open-source engineer working across databases, CLIs, and developer tooling. siphon is
            one of several projects — the rest live on GitHub.
          </p>
          <div className="mt-6">
            <ExtLink
              href={DEVELOPER.github}
              className="inline-flex rounded-lg border border-[var(--line)] px-4 py-2 text-sm text-[var(--paper)] no-underline transition-colors hover:border-[var(--flow)] hover:no-underline"
            >
              View @{DEVELOPER.handle} on GitHub ↗
            </ExtLink>
          </div>

          {/* Stats: a shimmering number over each (known) label while loading,
              real counts when ready. On error `tiles` is empty and nothing shows. */}
          {loading ? (
            <div className="mt-8 flex gap-8">
              {STAT_LABELS.map((label) => (
                <div key={label}>
                  <Skeleton className="h-7 w-14" />
                  <div className="mt-1 text-xs tracking-widest text-[var(--muted)] uppercase">
                    {label}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            tiles.length > 0 && (
              <div className="mt-8 flex gap-8">
                {tiles.map((t) => (
                  <div key={t.label}>
                    <div className="font-mono text-2xl font-bold text-[var(--paper)] tabular-nums">
                      {t.value.toLocaleString()}
                    </div>
                    <div className="mt-1 text-xs tracking-widest text-[var(--muted)] uppercase">
                      {t.label}
                    </div>
                  </div>
                ))}
              </div>
            )
          )}
        </div>

        {/* Top repos by stars. Skeleton grid while loading, real cards when
            ready; on error this whole column is omitted and the credit stands
            alone. */}
        {loading ? (
          <div>
            <p className="mb-3 font-mono text-xs tracking-widest text-[var(--muted)] uppercase">
              Top repositories
            </p>
            <div className="grid gap-px overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--line)] sm:grid-cols-2">
              {[0, 1, 2, 3].map((i) => (
                <div key={i} className="bg-[var(--ink)] p-4">
                  <div className="flex items-center justify-between gap-2">
                    <Skeleton className="h-4 w-32" />
                    <Skeleton className="h-3 w-8" />
                  </div>
                  <Skeleton className="mt-3 h-3 w-full" />
                  <Skeleton className="mt-1.5 h-3 w-2/3" />
                </div>
              ))}
            </div>
          </div>
        ) : (
          topRepos.length > 0 && (
            <div>
              <p className="mb-3 font-mono text-xs tracking-widest text-[var(--muted)] uppercase">
                Top repositories
              </p>
              <div className="grid gap-px overflow-hidden rounded-xl border border-[var(--line)] bg-[var(--line)] sm:grid-cols-2">
                {topRepos.map((r) => (
                  <ExtLink
                    key={r.name}
                    href={r.url}
                    className="block bg-[var(--ink)] p-4 no-underline transition-colors hover:bg-[var(--ink-2)] hover:no-underline"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <span className="truncate font-mono text-sm font-semibold text-[var(--paper)]">
                        {GH_OWNER}/{r.name}
                      </span>
                      <span className="shrink-0 font-mono text-xs text-[var(--amber)]">
                        ★ {r.stars.toLocaleString()}
                      </span>
                    </div>
                    {r.description && (
                      <p className="mt-2 line-clamp-2 text-xs leading-relaxed text-[var(--muted)]">
                        {r.description}
                      </p>
                    )}
                  </ExtLink>
                ))}
              </div>
            </div>
          )
        )}
      </div>
    </section>
  );
}
