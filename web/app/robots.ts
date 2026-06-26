import type { MetadataRoute } from "next";
import { SITE_URL } from "@/lib/site";

// Allow everything and point crawlers at the sitemap. Keeping this explicit
// (rather than relying on defaults) makes the indexing intent unambiguous.
export default function robots(): MetadataRoute.Robots {
  return {
    rules: { userAgent: "*", allow: "/" },
    sitemap: `${SITE_URL}/sitemap.xml`,
    host: SITE_URL,
  };
}
