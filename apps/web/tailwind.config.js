/** @type {import('tailwindcss').Config} */
module.exports = {
  darkMode: ["class"],
  content: [
    "./src/pages/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/components/**/*.{js,ts,jsx,tsx,mdx}",
    "./src/app/**/*.{js,ts,jsx,tsx,mdx}",
  ],
  theme: {
    extend: {
      colors: {
        border: "hsl(var(--border))",
        "border-strong": "hsl(var(--border-strong))",
        input: "hsl(var(--input))",
        ring: "hsl(var(--ring))",
        background: "hsl(var(--background))",
        foreground: "hsl(var(--foreground))",
        surface: {
          DEFAULT: "hsl(var(--surface))",
          2: "hsl(var(--surface-2))",
        },
        faint: "hsl(var(--faint))",
        muted: {
          DEFAULT: "hsl(var(--muted))",
          foreground: "hsl(var(--muted-foreground))",
        },
        // Semantic — kept separate from the accent hue.
        good: "hsl(var(--good))",
        warn: "hsl(var(--warn))",
        crit: "hsl(var(--crit))",
        info: "hsl(var(--info))",
        sidebar: "hsl(var(--surface))",
        accent: {
          DEFAULT: "#534AB7",
          hover: "#3C3489",
          ink: "#3E378F",
          light: "#EEEDFE",
          muted: "#AFA9EC",
          bright: "#8C84F5",
        },
      },
      fontFamily: {
        sans: ["var(--font-sans)", "Inter", "system-ui", "sans-serif"],
        mono: ["var(--font-mono)", "JetBrains Mono", "ui-monospace", "monospace"],
      },
      boxShadow: {
        card: "0 1px 2px rgba(21,26,31,.04), 0 8px 24px -12px rgba(21,26,31,.14)",
      },
    },
  },
  plugins: [],
};
