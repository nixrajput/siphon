// A shimmering placeholder block, sized by className. Used while GitHub data
// loads so the UI signals "fetching" instead of silently popping in. aria-hidden
// + a screen-reader "Loading" live region keep it accessible without reading out
// empty boxes.
export function Skeleton({ className = "" }: { className?: string }) {
  return <span aria-hidden className={`skeleton block ${className}`} />;
}
