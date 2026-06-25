import type { Metadata } from "next";
import "./globals.css";

export const metadata: Metadata = {
  title: "siphon — sync any database, anywhere",
  description:
    "A single binary that turns the painful sprawl of pg_dump → pg_restore shell scripts into a guided, observable workflow — backup, restore, sync, incremental, cross-engine, and CDC across PostgreSQL, MySQL, and MariaDB.",
};

export default function RootLayout({ children }: { children: React.ReactNode }) {
  return (
    <html lang="en">
      <body>{children}</body>
    </html>
  );
}
