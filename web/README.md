# siphon web — landing page & docs

The marketing landing page and documentation site for siphon, built with
Next.js (App Router) + Tailwind and deployed on Vercel. The docs pages render
the repository's own `docs/*.md` at build time, so the site never drifts from
the shipped documentation.

## Develop

```bash
cd web
npm install
npm run dev           # http://localhost:3000
npm run build         # production build (also what Vercel runs)

npm run lint          # ESLint
npm run lint:fix      # ESLint, autofix
npm run format        # Prettier, write
npm run format:check  # Prettier, check only (used by the pre-push hook)
```

The docs pipeline (`lib/docs.ts`) reads `../docs/*.md` relative to `web/`, so
run commands from inside `web/` with the repo checked out around it.

## Pre-push hook (lint + format gate)

A committed git hook (`.githooks/pre-push`) runs `npm run lint` and
`npm run format:check` before any push that touches `web/`, and **aborts the
push if either fails**. Pushes that don't change `web/` skip the check (so
Go-only pushes stay fast). Enable it once per clone — from the repo root:

```bash
make hooks            # = git config core.hooksPath .githooks
```

Bypass in an emergency with `git push --no-verify`. The repo root also exposes
`make web-lint` and `make web-format` as shortcuts into this app.

## Deploy (Vercel) — owner setup

Vercel deploys from this subdirectory via its git integration (no GitHub Action
needed). One-time setup on the Vercel account:

1. **Import the repo** into a new Vercel project.
2. Set **Root Directory** = `web`.
3. Framework preset: **Next.js** (auto-detected). Build command `next build`,
   output handled by Vercel.
4. **Custom domain** — add `siphon.nixrajput.com` to the project and point a
   CNAME at Vercel. The site's canonical URL, sitemap, robots, and Open Graph
   tags are already set to this domain (`lib/site.ts` → `SITE_URL`). The install
   command uses the raw GitHub URL, so no path rewrite is needed.

Pushes to `main` then deploy production; PRs get preview deployments.

## SEO / indexing

- Canonical URL, Open Graph + Twitter cards, and JSON-LD (`SoftwareApplication`
  - `Person`) are emitted from `app/layout.tsx`, keyed off `SITE_URL`.
- `app/sitemap.ts` and `app/robots.ts` generate `/sitemap.xml` + `/robots.txt`
  at build time; the sitemap is derived from the same `docNav()` source the
  pages render from, so it never drifts.
- `public/og.svg` is the share image. **Note:** several social crawlers don't
  rasterize SVG OG images — swap in a 1200×630 PNG (or an `opengraph-image.tsx`
  route) before launch if rich social unfurls matter.
- After deploy, submit the sitemap in Google Search Console for the domain.

## Phase H release provisioning checklist (owner)

The release tooling in the repo root (`.goreleaser.yaml`,
`.github/workflows/release.yml`, `scripts/install.sh`) is wired but needs these
account-level actions before a `v1.0.0` release publishes everywhere:

- [ ] **Tag and push `v1.0.0`** — triggers the release workflow (cross-platform
      binaries, checksums, cosign-keyless signatures, GitHub Release). This
      alone works with no further setup; `skip_upload: auto` means the brew/scoop
      steps no-op until their repos+tokens exist.
- [ ] **Homebrew tap** — create the public repo `nixrajput/homebrew-siphon`, mint
      a PAT with `repo` scope on it, and add it as the `HOMEBREW_TAP_GITHUB_TOKEN`
      secret on this repo. Then `brew install nixrajput/siphon/siphon` works.
- [ ] **Scoop bucket** — create `nixrajput/scoop-siphon`, add a PAT as
      `SCOOP_TAP_GITHUB_TOKEN`.
- [ ] **Actions permissions** — ensure the repo's Actions can use
      `contents: write` + `id-token: write` (already requested in the workflow;
      org settings may need to allow it).
- [ ] **Vercel** — the steps above, to publish this site.
