import type { MetadataRoute } from "next";
import { SITE_URL } from "@/lib/site";
import { docNav } from "@/lib/docs";

// Build the sitemap from the same nav source the site renders from, so every
// doc route is listed and the sitemap can't drift from the actual pages.
export default function sitemap(): MetadataRoute.Sitemap {
  const docs = docNav().map((d) => ({
    url: `${SITE_URL}/docs/${d.slug}`,
    changeFrequency: "monthly" as const,
    priority: 0.7,
  }));
  return [
    { url: SITE_URL, changeFrequency: "weekly", priority: 1 },
    { url: `${SITE_URL}/docs`, changeFrequency: "monthly", priority: 0.8 },
    ...docs,
  ];
}
