/** @type {import('tailwindcss').Config} */
export default {
  content: [
    "./index.html",
    "./src/**/*.{js,ts,jsx,tsx}",
  ],
  theme: {
    extend: {
      colors: {
        // Primary purple from banyan logo
        banyan: {
          50: '#faf5fc',
          100: '#f3e8f7',
          200: '#e8d4f0',
          300: '#d5b3e3',
          400: '#bb86d1',
          500: '#9d5bb8',
          600: '#6d3b7c',  // Primary purple
          700: '#5a2e68',  // Dark purple
          800: '#4a2656',
          900: '#3d2147',
        },
        // Orange/amber accent from tree trunk
        accent: {
          50: '#fef7f0',
          100: '#fdeee0',
          200: '#fbd9b8',
          300: '#f8bc85',
          400: '#f49550',
          500: '#cd4e34',  // Primary orange
          600: '#b83d2a',
          700: '#992d22',
          800: '#7a241f',
          900: '#651f1c',
        },
        // Golden/cream highlights
        gold: {
          50: '#fffef7',
          100: '#fefce8',
          200: '#fef9c3',
          300: '#f8dd99',  // Primary gold
          400: '#f5d067',
          500: '#eab308',
          600: '#ca8a04',
          700: '#a16207',
          800: '#854d0e',
          900: '#713f12',
        }
      }
    },
  },
  plugins: [],
}

