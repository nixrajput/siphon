// Single source of truth for cross-page constants: the canonical site URL (the
// custom domain), the GitHub repo, and the developer's references. Components
// and metadata import from here so the domain/links live in one place.

export const SITE_URL = "https://siphon.nixrajput.com";
export const SITE_NAME = "siphon";
export const SITE_TAGLINE = "Sync any database, anywhere.";
export const SITE_DESCRIPTION =
  "A single binary that turns the painful sprawl of pg_dump → pg_restore shell scripts into a guided, observable workflow — backup, restore, sync, incremental, cross-engine, and CDC across PostgreSQL, MySQL, and MariaDB.";

// GitHub owner + repo, used for live stats and links.
export const GH_OWNER = "nixrajput";
export const GH_REPO = "siphon";
export const REPO_URL = `https://github.com/${GH_OWNER}/${GH_REPO}`;

// The one canonical install command shown across the site. Raw GitHub so it
// works today without any domain-routing setup (matches the repo README).
export const INSTALL_CMD =
  "curl -fsSL https://raw.githubusercontent.com/nixrajput/siphon/main/scripts/install.sh | sh";

// Developer references for the footer + credit.
export const DEVELOPER = {
  name: "Nikhil Rajput",
  handle: "nixrajput",
  portfolio: "https://nixrajput.com",
  github: "https://github.com/nixrajput",
};

// Engines siphon speaks, used by the hero version pill fallback + engines strip.
export const ENGINES = ["PostgreSQL", "MySQL", "MariaDB"] as const;
