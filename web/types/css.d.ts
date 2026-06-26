// Ambient declarations for stylesheet imports. Next.js normally provides these
// via its bundled types (referenced from next-env.d.ts), but declaring them here
// makes a bare side-effect import like `import "./globals.css"` resolve even when
// an editor's TS server or a standalone `tsc` run hasn't loaded Next's ambient
// types yet — fixing "Cannot find module or type declarations for side-effect
// import of './globals.css'."

// Plain global stylesheet, imported only for its side effect.
declare module "*.css";

// CSS Modules: `import styles from "./x.module.css"` returns a class-name map.
declare module "*.module.css" {
  const classes: { readonly [key: string]: string };
  export default classes;
}
