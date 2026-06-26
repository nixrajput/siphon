// Tailwind v4 ships its PostCSS plugin as a separate package and handles vendor
// prefixing + nesting internally (Lightning CSS), so autoprefixer is no longer
// needed.
const config = {
  plugins: { "@tailwindcss/postcss": {} },
};

export default config;
