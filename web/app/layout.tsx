import type { Metadata } from "next";
import "./globals.css";
import {
  SITE_URL,
  SITE_NAME,
  SITE_TAGLINE,
  SITE_DESCRIPTION,
  REPO_URL,
  DEVELOPER,
} from "@/lib/site";

// metadataBase makes every relative OG/canonical URL resolve against the custom
// domain, so social cards and search engines see siphon.nixrajput.com — not a
// Vercel preview host. Title uses a template so child pages read "X — siphon".
export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: `${SITE_NAME} — sync any database, anywhere`,
    template: `%s — ${SITE_NAME}`,
  },
  description: SITE_DESCRIPTION,
  applicationName: SITE_NAME,
  keywords: [
    "database backup",
    "database sync",
    "postgresql backup",
    "mysql backup",
    "mariadb backup",
    "pg_dump alternative",
    "change data capture",
    "cdc",
    "cross-engine migration",
    "incremental backup",
    "cli",
    "golang",
    "devops",
  ],
  authors: [{ name: DEVELOPER.name, url: DEVELOPER.portfolio }],
  creator: DEVELOPER.name,
  alternates: { canonical: "/" },
  openGraph: {
    type: "website",
    siteName: SITE_NAME,
    url: SITE_URL,
    title: `${SITE_NAME} — ${SITE_TAGLINE}`,
    description: SITE_DESCRIPTION,
    images: [{ url: "/og.svg", width: 1200, height: 630, alt: `${SITE_NAME} — ${SITE_TAGLINE}` }],
  },
  twitter: {
    card: "summary_large_image",
    title: `${SITE_NAME} — ${SITE_TAGLINE}`,
    description: SITE_DESCRIPTION,
    images: ["/og.svg"],
  },
  robots: {
    index: true,
    follow: true,
    googleBot: { index: true, follow: true, "max-image-preview": "large" },
  },
};

// JSON-LD: tells Google this page documents a downloadable developer tool. The
// SoftwareApplication + Person graph is what powers a rich result and ties the
// project to its author (nixrajput) for entity/"geo" understanding.
const JSON_LD = {
  "@context": "https://schema.org",
  "@graph": [
    {
      "@type": "SoftwareApplication",
      name: SITE_NAME,
      url: SITE_URL,
      description: SITE_DESCRIPTION,
      applicationCategory: "DeveloperApplication",
      operatingSystem: "Linux, macOS, Windows",
      offers: { "@type": "Offer", price: "0", priceCurrency: "USD" },
      license: "https://opensource.org/licenses/MIT",
      codeRepository: REPO_URL,
      author: {
        "@type": "Person",
        name: DEVELOPER.name,
        alternateName: DEVELOPER.handle,
        url: DEVELOPER.portfolio,
      },
    },
    {
      "@type": "WebSite",
      name: SITE_NAME,
      url: SITE_URL,
    },
  ],
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>
        {children}
        <script
          type="application/ld+json"
          // Structured data is static, build-time JSON — safe to inline.
          dangerouslySetInnerHTML={{ __html: JSON.stringify(JSON_LD) }}
        />
      </body>
    </html>
  );
}
