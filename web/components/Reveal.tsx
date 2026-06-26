"use client";

import { useEffect, useRef, useState } from "react";

// Scroll-reveal as a PROGRESSIVE ENHANCEMENT: the server renders children fully
// visible (no hidden state), so without JS — or if the observer never fires —
// content is always present. Only after mount do we add the .reveal class
// (initially hidden) and observe; once in view, .is-in animates it up. This
// avoids the classic "JS-off leaves content invisible" bug.
export function Reveal({
  children,
  delay = 0,
  className = "",
}: {
  children: React.ReactNode;
  delay?: number;
  className?: string;
}) {
  const ref = useRef<HTMLDivElement>(null);
  const [armed, setArmed] = useState(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;

    // Respect reduced motion: stay plainly visible, never animate.
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    // Only arm the hidden state if we can actually reveal it again — without
    // IntersectionObserver, leave the visible server render untouched.
    if (typeof IntersectionObserver === "undefined") return;

    // Deliberate one-shot setState in effect: this is the progressive-enhancement
    // pivot — the server renders visible (no .reveal), and only after mount, once
    // JS + IntersectionObserver are confirmed, do we arm the hidden state. The
    // single extra render is intended; refactoring it away risks the no-JS flash.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    setArmed(true);
    const obs = new IntersectionObserver(
      (entries) => {
        for (const e of entries) {
          if (e.isIntersecting) {
            e.target.classList.add("is-in");
            obs.unobserve(e.target);
          }
        }
      },
      { rootMargin: "0px 0px -10% 0px", threshold: 0.1 },
    );
    obs.observe(el);
    return () => obs.disconnect();
  }, []);

  return (
    <div
      ref={ref}
      className={`${armed ? "reveal" : ""} ${className}`}
      style={delay ? ({ "--rise-delay": `${delay}ms` } as React.CSSProperties) : undefined}
    >
      {children}
    </div>
  );
}
