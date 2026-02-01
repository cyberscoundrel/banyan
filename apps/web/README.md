# Banyan Web Documentation

A modern, minimal documentation website for Banyan - the open-source peer-to-peer networking multitool. Built with React, TypeScript, Webpack, Tailwind CSS, and React Router.

## Features

- 📱 Responsive design with elegant minimal styling
- 🌓 Dark/Light mode toggle
- 🧭 Dynamic sidebar navigation (main site + documentation)
- 📚 Comprehensive documentation structure with topics and subtopics
- 📦 Download section for releases
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
apps/web/
├── public/
│   ├── index.html
│   └── fig.svg
├── src/
│   ├── components/
│   │   ├── Header.tsx        # Header with logo and theme toggle
│   │   ├── Layout.tsx        # Main layout wrapper with conditional sidebar
│   │   ├── Sidebar.tsx       # Main navigation sidebar
│   │   └── DocsSidebar.tsx   # Documentation navigation sidebar
│   ├── contexts/
│   │   └── ThemeContext.tsx  # Theme management
│   ├── pages/
│   │   ├── Home.tsx          # Overview page
│   │   ├── Usage.tsx         # Usage documentation
│   │   ├── API.tsx           # API reference
│   │   ├── Examples.tsx      # Usage examples
│   │   ├── Download.tsx      # Download releases
│   │   └── Docs.tsx          # Documentation hub (placeholder)
│   ├── App.tsx               # Main app component with routing
│   ├── index.tsx             # Entry point
│   └── index.css             # Global styles and Tailwind
├── package.json
├── webpack.config.js
├── tailwind.config.js
├── tsconfig.json
└── README.md
```

## Site Structure

The website is organized into two main sections:

### Main Site
- **Overview**: Introduction to Banyan and its use cases
- **Download**: Release downloads and installation instructions
- **Usage**: Basic usage and command-line options
- **API Reference**: Complete REST API documentation
- **Examples**: Common usage scenarios and code samples

### Documentation Section (`/docs`)
- Comprehensive documentation with expandable topics and subtopics
- Dedicated documentation sidebar with collapsible sections
- Placeholder structure ready for content population
- Topics include: Getting Started, Core Concepts, Guides, API Reference, Advanced Topics, and Contributing

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

## About Banyan

Banyan is an open-source peer-to-peer networking multitool that transforms traditional web applications and microservices into decentralized systems without requiring code changes. Built on LibP2P, it provides HTTP proxy functionality, peer discovery, service management, and real-time monitoring.

## Deployment

The built site is a static SPA that can be deployed to:
- GitHub Pages
- Netlify
- Vercel
- Any static hosting service

Make sure to configure the hosting service to handle client-side routing (redirect all routes to index.html).

