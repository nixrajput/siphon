// The hero artifact: a real, syntax-true siphon session. Not a marketing
// illustration — the most characteristic thing in this product's world is the
// command transcript itself, so that IS the hero. Lines are colored by role
// (prompt, command, flow output) using the palette's flow accent.
type Line =
  | { kind: "cmd"; text: string }
  | { kind: "out"; text: string }
  | { kind: "flow"; text: string };

const SESSION: Line[] = [
  { kind: "cmd", text: "siphon backup prod" },
  { kind: "out", text: "dumping  ████████████  done" },
  { kind: "flow", text: "wrote dump 01JC8…  (sha256 verified)" },
  { kind: "cmd", text: "siphon sync prod staging" },
  { kind: "flow", text: "prod ──▶ staging   streaming, no temp file" },
  { kind: "out", text: "restored  •  42 tables  •  1.3 GB" },
  { kind: "cmd", text: "siphon cdc prod replica" },
  { kind: "flow", text: "following changes…  applied 318 (live)" },
];

export function Terminal() {
  return (
    <div className="overflow-hidden rounded-xl border border-[var(--line)] bg-[#0a111e] shadow-2xl">
      <div className="flex items-center gap-2 border-b border-[var(--line)] px-4 py-3">
        <span className="h-3 w-3 rounded-full bg-[#ff5f56]" aria-hidden />
        <span className="h-3 w-3 rounded-full bg-[#ffbd2e]" aria-hidden />
        <span className="h-3 w-3 rounded-full bg-[#27c93f]" aria-hidden />
        <span className="ml-3 font-mono text-xs text-[var(--muted)]">siphon — zsh</span>
      </div>
      <pre className="overflow-x-auto px-4 py-4 font-mono text-[0.82rem] leading-relaxed">
        {SESSION.map((line, i) => (
          <div key={i}>
            {line.kind === "cmd" && (
              <span>
                <span className="text-[var(--amber)]">$ </span>
                <span className="text-[var(--paper)]">{line.text}</span>
              </span>
            )}
            {line.kind === "out" && (
              <span className="text-[var(--muted)]">{line.text}</span>
            )}
            {line.kind === "flow" && (
              <span className="text-[var(--flow)]">{line.text}</span>
            )}
          </div>
        ))}
      </pre>
    </div>
  );
}
