"use client";

import { INSTALL_CMD, REPO_URL } from "@/lib/site";
import { useRepoStats } from "@/components/useGitHub";
import { InstallCommand } from "@/components/InstallCommand";
import { ExtLink } from "@/components/ExtLink";
import { Skeleton } from "@/components/Skeleton";

// The hero's action block: the live latest version sits right above the
// one-line install, so "what's current" and "how to get it" read together.
// Version + star count are live; both degrade to quiet fallbacks on fetch
// failure so the block is never broken.
export function HeroInstall() {
  const { status, version, stars } = useRepoStats();

  return (
    <div>
      <div className="mb-3 flex flex-wrap items-center gap-x-4 gap-y-1 font-mono text-xs text-[var(--muted)]">
        <ExtLink
          href={`${REPO_URL}/releases`}
          className="inline-flex items-center gap-2 no-underline hover:text-[var(--paper)]"
        >
          <span className="pulse-dot h-1.5 w-1.5 rounded-full bg-[var(--flow)]" aria-hidden />
          {/* Show the real tag + "latest release" once one exists; until then show
              neutral "unreleased" rather than claiming a version that isn't out. */}
          <span className="text-[var(--flow)]">{version ?? "unreleased"}</span>
          {version && <span>latest release</span>}
        </ExtLink>
        {/* Star count is genuinely unknown until the fetch lands: shimmer while
            loading, real count when ready, nothing on error or zero stars. */}
        {status === "loading" ? (
          <Skeleton className="h-3 w-28" />
        ) : (
          stars !== null &&
          stars > 0 && (
            <ExtLink href={REPO_URL} className="no-underline hover:text-[var(--paper)]">
              ★ {stars.toLocaleString()} on GitHub
            </ExtLink>
          )
        )}
      </div>
      <InstallCommand command={INSTALL_CMD} />
    </div>
  );
}
