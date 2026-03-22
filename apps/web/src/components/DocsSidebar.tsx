import React, { useState } from 'react';
import { NavLink, Link } from 'react-router-dom';
import { ChevronDown, ChevronRight, BookOpen, Rocket, Lightbulb, Code2, Wrench, GraduationCap, ArrowLeft } from 'lucide-react';

interface DocSection {
  name: string;
  icon: React.ElementType;
  items: {
    name: string;
    href: string;
  }[];
}

const docSections: DocSection[] = [
  {
    name: 'Getting Started',
    icon: Rocket,
    items: [
      { name: 'Introduction', href: '/docs' },
      { name: 'Installation', href: '/docs/getting-started/installation' },
      { name: 'Quick Start', href: '/docs/getting-started/quick-start' },
      { name: 'Basic Concepts', href: '/docs/getting-started/concepts' },
    ],
  },
  {
    name: 'Core Concepts',
    icon: Lightbulb,
    items: [
      { name: 'Peer-to-Peer Networking', href: '/docs/concepts/p2p-networking' },
      { name: 'Peer Discovery', href: '/docs/concepts/peer-discovery' },
      { name: 'Service Discovery', href: '/docs/concepts/service-discovery' },
      { name: 'Cryptographic Identity', href: '/docs/concepts/cryptographic-identity' },
      { name: 'NAT Traversal', href: '/docs/concepts/nat-traversal' },
    ],
  },
  {
    name: 'Guides',
    icon: BookOpen,
    items: [
      { name: 'Service Mesh Setup', href: '/docs/guides/service-mesh' },
      { name: 'Microservices', href: '/docs/guides/microservices' },
      { name: 'IoT Networks', href: '/docs/guides/iot-networks' },
      { name: 'Secure Communication', href: '/docs/guides/secure-communication' },
      { name: 'Monitoring & Debugging', href: '/docs/guides/monitoring' },
    ],
  },
  {
    name: 'API Reference',
    icon: Code2,
    items: [
      { name: 'Package Reference', href: '/docs/api' },
      { name: 'REST API', href: '/docs/api/rest' },
      { name: 'WebSocket Events', href: '/docs/api/websocket' },
      { name: 'CLI Reference', href: '/docs/api/cli' },
      { name: 'Configuration', href: '/docs/api/configuration' },
    ],
  },
  {
    name: 'Advanced',
    icon: Wrench,
    items: [
      { name: 'Custom Routing', href: '/docs/advanced/custom-routing' },
      { name: 'Performance Tuning', href: '/docs/advanced/performance' },
      { name: 'Security Best Practices', href: '/docs/advanced/security' },
      { name: 'Troubleshooting', href: '/docs/advanced/troubleshooting' },
    ],
  },
  {
    name: 'Contributing',
    icon: GraduationCap,
    items: [
      { name: 'How to Contribute', href: '/docs/contributing/how-to' },
      { name: 'Development Setup', href: '/docs/contributing/development' },
      { name: 'Code Guidelines', href: '/docs/contributing/guidelines' },
    ],
  },
];

const DocsSidebar: React.FC = () => {
  const [expandedSections, setExpandedSections] = useState<Set<string>>(
    new Set(['Getting Started'])
  );

  const toggleSection = (sectionName: string) => {
    setExpandedSections((prev) => {
      const newSet = new Set(prev);
      if (newSet.has(sectionName)) {
        newSet.delete(sectionName);
      } else {
        newSet.add(sectionName);
      }
      return newSet;
    });
  };

  return (
    <aside className="w-64 bg-white dark:bg-gray-900 h-full">
      <nav className="px-4 pt-4 pb-8 space-y-1">
        {/* Back to main navigation */}
        <Link 
          to="/" 
          className="flex items-center space-x-2 px-3 py-2 mb-4 rounded-lg text-sm font-medium text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800 hover:text-gray-900 dark:hover:text-white transition-colors border border-gray-200 dark:border-gray-700"
        >
          <ArrowLeft className="w-4 h-4" />
          <span>Back to Home</span>
        </Link>
        
        <div className="mb-4 px-3 text-xs font-semibold text-gray-500 dark:text-gray-400 uppercase tracking-wider">
          Documentation
        </div>
        {docSections.map((section) => {
          const Icon = section.icon;
          const isExpanded = expandedSections.has(section.name);
          
          return (
            <div key={section.name}>
              <button
                onClick={() => toggleSection(section.name)}
                className="w-full flex items-center justify-between px-3 py-2 rounded-lg text-sm font-medium text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-800 transition-colors"
              >
                <div className="flex items-center space-x-2">
                  <Icon className="w-4 h-4" />
                  <span>{section.name}</span>
                </div>
                {isExpanded ? (
                  <ChevronDown className="w-4 h-4" />
                ) : (
                  <ChevronRight className="w-4 h-4" />
                )}
              </button>
              
              {isExpanded && (
                <div className="ml-6 mt-1 space-y-1">
                  {section.items.map((item) => (
                    <NavLink
                      key={item.href}
                      to={item.href}
                      end={item.href === '/docs'}
                      className={({ isActive }) =>
                        `block px-3 py-1.5 rounded-lg text-sm transition-colors ${
                          isActive
                            ? 'bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300 font-medium'
                            : 'text-gray-600 dark:text-gray-400 hover:bg-gray-100 dark:hover:bg-gray-800'
                        }`
                      }
                    >
                      {item.name}
                    </NavLink>
                  ))}
                </div>
              )}
            </div>
          );
        })}
      </nav>
    </aside>
  );
};

export default DocsSidebar;

