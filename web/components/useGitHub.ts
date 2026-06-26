"use client";

import { useEffect, useState } from "react";
import { GH_OWNER, GH_REPO } from "@/lib/site";
import { ghFetch } from "@/components/githubCache";

// Live GitHub data, fetched client-side through a cached + deduped layer
// (githubCache.ts) so a visitor stays well under the unauthenticated
// 60 req/hr/IP limit. The hooks ALWAYS degrade gracefully: on any failure
// (rate limit / offline / no release yet) fields are null and callers fall back
// to static copy — never a broken "NaN" or error box.
//
// `status` distinguishes the three async states so callers can render a
// skeleton while loading, real data when ready, and a quiet fallback on error.
// (Nullable fields alone can't tell "still fetching" from "gave up".)
export type FetchStatus = "loading" | "ready" | "error";

export type RepoStats = {
  status: FetchStatus;
  stars: number | null;
  forks: number | null;
  // The latest release tag (e.g. "v1.0.0"), or null if none published yet.
  version: string | null;
};

export type DevStats = {
  status: FetchStatus;
  followers: number | null;
  publicRepos: number | null;
  // Sum of stars across the developer's own (non-fork) repos. Capped at the
  // first 100 repos (one page) — fine here; would undercount above 100.
  totalStars: number | null;
  topRepos: { name: string; stars: number; description: string | null; url: string }[];
};

// Shared shapes for the GitHub responses we read.
type GhRepo = {
  name: string;
  stargazers_count: number;
  forks_count: number;
  description: string | null;
  html_url: string;
  fork: boolean;
};
type GhUser = { followers: number; public_repos: number };
type GhRelease = { tag_name: string };

// Canonical URLs. Both hooks read the SAME repos URL, so the cache/dedup layer
// turns their two requests into one shared network call.
const REPOS_URL = `https://api.github.com/users/${GH_OWNER}/repos?per_page=100&sort=updated`;
const USER_URL = `https://api.github.com/users/${GH_OWNER}`;
const RELEASE_URL = `https://api.github.com/repos/${GH_OWNER}/${GH_REPO}/releases/latest`;

// siphon's own repo stats, sourced from the developer's repos list (so no
// separate repos/{owner}/{repo} call), plus the latest release tag. Two calls,
// one of which (repos) is shared with useDevStats.
export function useRepoStats(): RepoStats {
  const [stats, setStats] = useState<RepoStats>({
    status: "loading",
    stars: null,
    forks: null,
    version: null,
  });

  useEffect(() => {
    let alive = true;
    (async () => {
      const [repos, release] = await Promise.all([
        ghFetch<GhRepo[]>(REPOS_URL),
        // A 404 here just means no release is published yet — not an error.
        ghFetch<GhRelease>(RELEASE_URL),
      ]);
      if (!alive) return;
      // If the repos list is unreachable (403 rate limit / offline), it's a real
      // error — we can't know siphon's stars.
      if (!Array.isArray(repos)) {
        setStats({ status: "error", stars: null, forks: null, version: null });
        return;
      }
      const self = repos.find((r) => r.name.toLowerCase() === GH_REPO.toLowerCase());
      setStats({
        status: "ready",
        stars: self?.stargazers_count ?? null,
        forks: self?.forks_count ?? null,
        version: release?.tag_name ?? null,
      });
    })();
    return () => {
      alive = false;
    };
  }, []);

  return stats;
}

// The developer's follower/repo counts + top repos by stars + total stars.
export function useDevStats(): DevStats {
  const [stats, setStats] = useState<DevStats>({
    status: "loading",
    followers: null,
    publicRepos: null,
    totalStars: null,
    topRepos: [],
  });

  useEffect(() => {
    let alive = true;
    (async () => {
      const [user, repos] = await Promise.all([
        ghFetch<GhUser>(USER_URL),
        ghFetch<GhRepo[]>(REPOS_URL), // shared with useRepoStats via the cache
      ]);
      if (!alive) return;
      // Both unreachable → surface as error so callers drop the skeleton and
      // fall back to a plain credit.
      if (!user && !Array.isArray(repos)) {
        setStats({
          status: "error",
          followers: null,
          publicRepos: null,
          totalStars: null,
          topRepos: [],
        });
        return;
      }
      const list = Array.isArray(repos) ? repos : [];
      const own = list.filter((r) => !r.fork);
      const totalStars = list.length ? own.reduce((sum, r) => sum + r.stargazers_count, 0) : null;
      const topRepos = own
        .sort((a, b) => b.stargazers_count - a.stargazers_count)
        .slice(0, 4)
        .map((r) => ({
          name: r.name,
          stars: r.stargazers_count,
          description: r.description,
          url: r.html_url,
        }));
      setStats({
        status: "ready",
        followers: user?.followers ?? null,
        publicRepos: user?.public_repos ?? null,
        totalStars,
        topRepos,
      });
    })();
    return () => {
      alive = false;
    };
  }, []);

  return stats;
}
