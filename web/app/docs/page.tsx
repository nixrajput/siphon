import Link from "next/link";
import { docNav } from "@/lib/docs";

export default function DocsIndex() {
  const nav = docNav();
  return (
    <main>
      <h1>Documentation</h1>
      <ul>
        {nav.map((d) => (
          <li key={d.slug}>
            <Link href={`/docs/${d.slug}`}>{d.title}</Link>
          </li>
        ))}
      </ul>
    </main>
  );
}
