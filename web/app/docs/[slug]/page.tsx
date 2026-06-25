import { notFound } from "next/navigation";
import { docNav, getDoc } from "@/lib/docs";
import { Markdown } from "@/components/Markdown";

// Statically generate one page per doc at build time.
export function generateStaticParams() {
  return docNav().map((d) => ({ slug: d.slug }));
}

export async function generateMetadata({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const doc = getDoc(slug);
  return { title: doc ? `${doc.title} — siphon` : "siphon docs" };
}

export default async function DocPage({
  params,
}: {
  params: Promise<{ slug: string }>;
}) {
  const { slug } = await params;
  const doc = getDoc(slug);
  if (!doc) notFound();
  return (
    <main>
      <Markdown content={doc.content} />
    </main>
  );
}
