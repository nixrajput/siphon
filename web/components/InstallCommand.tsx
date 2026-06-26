"use client";

import { useEffect, useRef, useState } from "react";

// The single install action — the page's one amber moment. Click copies the
// command; the label confirms in place ("Copied") rather than via a separate
// toast, so the feedback stays where the eye already is.
export function InstallCommand({ command }: { command: string }) {
  const [copied, setCopied] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Clear a pending reset on unmount so it can't fire on a gone component.
  useEffect(
    () => () => {
      if (timer.current) clearTimeout(timer.current);
    },
    [],
  );

  async function copy() {
    try {
      await navigator.clipboard.writeText(command);
      setCopied(true);
      // Reset any in-flight timer so a rapid second click doesn't flip the
      // label back to "Copy" early.
      if (timer.current) clearTimeout(timer.current);
      timer.current = setTimeout(() => {
        setCopied(false);
        timer.current = null;
      }, 1600);
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
      <span aria-hidden className="text-[var(--amber)] select-none">
        $
      </span>
      <code className="flex-1 overflow-x-auto whitespace-nowrap text-[var(--paper)]">
        {command}
      </code>
      <span
        className={`text-xs tracking-widest uppercase select-none ${
          copied ? "text-[var(--flow)]" : "text-[var(--muted)] group-hover:text-[var(--amber)]"
        }`}
      >
        {copied ? "Copied" : "Copy"}
      </span>
    </button>
  );
}
