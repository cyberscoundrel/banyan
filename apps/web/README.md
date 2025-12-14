# Banyan Documentation Website

A modern, minimal documentation website for Banyan - the LibP2P HTTP Proxy Node. Built with React, TypeScript, Webpack, Tailwind CSS, and React Router.

## Features

- 📱 Responsive design with elegant minimal styling
- 🌓 Dark/Light mode toggle
- 🧭 Left sidebar navigation like a documentation site
- 📚 Complete documentation from markdown sources
- 📦 Download section for prerelease builds
- ⚡ Fast Webpack-based build system
- 🎨 Tailwind CSS for styling
- 🔷 TypeScript for type safety

## Getting Started

### Prerequisites

- Node.js 16+ 
- npm or yarn

### Installation

```bash
cd webpage
npm install
```

### Development

```bash
npm run dev
# or
npm start
```

The site will be available at `http://localhost:3000`

### Building for Production

```bash
npm run build
```

The built files will be in the `dist/` directory.

## Project Structure

```
webpage/
├── public/
│   ├── index.html
│   └── favicon.ico
├── src/
│   ├── components/
│   │   ├── Header.tsx        # Header with logo and theme toggle
│   │   ├── Layout.tsx        # Main layout wrapper
│   │   └── Sidebar.tsx       # Left navigation sidebar
│   ├── contexts/
│   │   └── ThemeContext.tsx  # Theme management
│   ├── pages/
│   │   ├── Home.tsx          # Overview page
│   │   ├── Installation.tsx  # Installation guide
│   │   ├── Usage.tsx         # Usage documentation
│   │   ├── API.tsx           # API reference
│   │   ├── Examples.tsx      # Usage examples
│   │   ├── Troubleshooting.tsx
│   │   ├── Future.tsx        # Future plans
│   │   └── Download.tsx      # Download prerelease builds
│   ├── App.tsx               # Main app component
│   ├── index.tsx             # Entry point
│   └── index.css             # Global styles and Tailwind
├── package.json
├── webpack.config.js
├── tailwind.config.js
├── tsconfig.json
└── README.md
```

## Content Sources

The documentation content is derived from the main `Banyan_Documentation.md` file and organized into logical sections:

- **Home**: Overview and capabilities
- **Installation**: Setup and configuration
- **Usage**: Basic usage and best practices  
- **API**: Complete API reference
- **Examples**: Usage examples and scenarios
- **Troubleshooting**: Common issues and solutions
- **Future**: Project roadmap and plans
- **Download**: Prerelease binary downloads

## Theming

The site supports both light and dark modes with:
- System preference detection
- Manual toggle in header
- Persistent theme selection
- Smooth transitions

## Navigation

Left sidebar navigation includes:
- Visual icons for each section
- Active state highlighting
- Clean, minimal styling
- Responsive behavior

## Logo Placeholder

There's a placeholder logo spot in the header (currently showing "B" in a colored square). Replace with your actual logo by updating the Header component.

## Customization

### Colors
Edit `tailwind.config.js` to customize the color scheme:

```js
colors: {
  primary: {
    // Your brand colors
  }
}
```

### Typography
The site uses Inter font family with JetBrains Mono for code. Update in `tailwind.config.js` and `public/index.html`.

### Content
Update the page components in `src/pages/` to modify the documentation content.

## Deployment

The built site is a static SPA that can be deployed to:
- GitHub Pages
- Netlify
- Vercel
- Any static hosting service

Make sure to configure the hosting service to handle client-side routing (redirect all routes to index.html).

