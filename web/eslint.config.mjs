// Flat ESLint config (ESLint 10 + Next 16). Replaces the legacy .eslintrc.json:
// `next lint` is deprecated in Next 16, so we run `eslint .` against this config.
// eslint-config-next ships flat-config arrays for its Core Web Vitals +
// TypeScript rule sets, which we spread in directly.
import nextCoreWebVitals from "eslint-config-next/core-web-vitals";
import nextTypeScript from "eslint-config-next/typescript";

const config = [
  { ignores: [".next/**", "node_modules/**", "out/**", "next-env.d.ts"] },
  ...nextCoreWebVitals,
  ...nextTypeScript,
];

export default config;
