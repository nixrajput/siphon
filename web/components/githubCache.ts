"use client";

// A tiny cached + deduped GET layer for the GitHub API, built to stay well under
// the unauthenticated 60 req/hr/IP limit:
//
//   • sessionStorage cache with a TTL — once a visitor loads the data, reloads
//     and in-session navigations reuse it instead of re-fetching.
//   • in-flight dedup — if two components request the same URL on the same load
//     (e.g. the hero and the developer section both need repo data), they share
//     ONE network request instead of firing two.
//   • a null cache return is still cached briefly so a 403/offline burst doesn't
//     hammer the API on every render.
//
// Returns the parsed JSON on 2xx, or null on any non-OK status (403 rate limit,
// 404 no-release, network error). Callers decide what null means.

const TTL_MS = 30 * 60 * 1000; // 30 min: fresh enough for stars/followers
const NEG_TTL_MS = 60 * 1000; // cache failures only briefly, then allow a retry
const PREFIX = "gh:"; // sessionStorage key namespace

type Cached = { at: number; ok: boolean; data: unknown };

// Promises for requests currently in flight, keyed by URL. Lets concurrent
// callers await the same fetch instead of starting their own.
const inflight = new Map<string, Promise<unknown>>();

function readCache(url: string): Cached | null {
  try {
    const raw = sessionStorage.getItem(PREFIX + url);
    if (!raw) return null;
    const c = JSON.parse(raw) as Cached;
    const ttl = c.ok ? TTL_MS : NEG_TTL_MS;
    if (Date.now() - c.at > ttl) {
      sessionStorage.removeItem(PREFIX + url);
      return null;
    }
    return c;
  } catch {
    return null; // private mode / quota / parse error — just skip the cache
  }
}

function writeCache(url: string, ok: boolean, data: unknown): void {
  try {
    const entry: Cached = { at: Date.now(), ok, data };
    sessionStorage.setItem(PREFIX + url, JSON.stringify(entry));
  } catch {
    /* storage unavailable — fine, we simply don't cache */
  }
}

// Fetch a GitHub API URL through the cache + dedup layer. Generic over the
// expected JSON shape; returns null on any failure.
export async function ghFetch<T>(url: string): Promise<T | null> {
  const hit = readCache(url);
  if (hit) return hit.data as T | null;

  const existing = inflight.get(url);
  if (existing) return existing as Promise<T | null>;

  const p = (async (): Promise<T | null> => {
    try {
      const res = await fetch(url, { headers: { Accept: "application/vnd.github+json" } });
      if (!res.ok) {
        writeCache(url, false, null);
        return null;
      }
      const data = (await res.json()) as T;
      writeCache(url, true, data);
      return data;
    } catch {
      writeCache(url, false, null);
      return null;
    } finally {
      inflight.delete(url);
    }
  })();

  inflight.set(url, p);
  return p;
}
