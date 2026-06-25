"use client";

import { useState } from "react";

// The single install action — the page's one amber moment. Click copies the
// command; the label confirms in place ("Copied") rather than via a separate
// toast, so the feedback stays where the eye already is.
export function InstallCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      setTimeout(() => setCopied(false), 1600);
    } catch {
      setCopied(false);
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      className="group flex w-full items-center gap-3 rounded-lg border border-[var(--line)] bg-[var(--ink-2)] px-4 py-3 text-left font-mono text-sm transition-colors hover:border-[var(--amber)]"
      aria-label={`Copy install command: ${command}`}
    >
      <span aria-hidden className="select-none text-[var(--amber)]">$</span>
      <code className="flex-1 overflow-x-auto whitespace-nowrap text-[var(--paper)]">
        {command}
      </code>
      <span
        className={`select-none text-xs uppercase tracking-widest ${
          copied ? "text-[var(--flow)]" : "text-[var(--muted)] group-hover:text-[var(--amber)]"
        }`}
      >
        {copied ? "Copied" : "Copy"}
      </span>
    </button>
  );
}
