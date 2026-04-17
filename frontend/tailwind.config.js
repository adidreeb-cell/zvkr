/** @type {import('tailwindcss').Config} */
export default {
  content: ["./index.html", "./src/**/*.{js,ts,jsx,tsx}"],
  darkMode: "class",
  theme: {
    extend: {
      fontFamily: {
        sans: ["Inter", "sans-serif"],
        mono: ["JetBrains Mono", "monospace"],
      },
      colors: {
        background: "#0a0a0a",
        surface: "#111111",
        border: "#333333",
        primary: "#ededed",
        primaryFg: "#0a0a0a",
        accent: "#3291ff",
      },
    },
  },
  plugins: [],
};
