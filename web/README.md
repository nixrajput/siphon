# siphon web — landing page & docs

The marketing landing page and documentation site for siphon, built with
Next.js (App Router) + Tailwind and deployed on Vercel. The docs pages render
the repository's own `docs/*.md` at build time, so the site never drifts from
the shipped documentation.

## Develop

```bash
cd web
npm install
npm run dev      # http://localhost:3000
npm run build    # production build (also what Vercel runs)
npm run lint
```

The docs pipeline (`lib/docs.ts`) reads `../docs/*.md` relative to `web/`, so
run commands from inside `web/` with the repo checked out around it.

## Deploy (Vercel) — owner setup

Vercel deploys from this subdirectory via its git integration (no GitHub Action
needed). One-time setup on the Vercel account:

1. **Import the repo** into a new Vercel project.
2. Set **Root Directory** = `web`.
3. Framework preset: **Next.js** (auto-detected). Build command `next build`,
   output handled by Vercel.
4. (Optional) Point the `siphon.dev` domain at the project; the install command
   on the landing page references `https://siphon.dev/install.sh`, so either map
   that path to the repo's `scripts/install.sh` (e.g. a redirect/rewrite) or
   update the command to the raw GitHub URL.

Pushes to `main` then deploy production; PRs get preview deployments.

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
