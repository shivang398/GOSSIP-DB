/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        background: '#0a0a0b',
        card: '#111113',
        primary: '#3b82f6',
        secondary: '#1e1e21',
        border: '#27272a',
      },
    },
  },
  plugins: [],
}
