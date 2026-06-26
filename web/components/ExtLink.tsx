// An external link: always opens in a new tab with a safe rel. Use this for any
// off-site URL (GitHub, releases, the developer's portfolio). Internal routes
// should use next/link instead, so they stay in the same tab as an SPA nav.
export function ExtLink({
  href,
  className,
  children,
  ...rest
}: React.AnchorHTMLAttributes<HTMLAnchorElement>) {
  return (
    <a href={href} target="_blank" rel="noopener noreferrer" className={className} {...rest}>
      {children}
    </a>
  );
}
