import React from 'react';
import { NavLink } from 'react-router-dom';
import { 
  Home, 
  Download, 
  BookOpen, 
  Terminal, 
  Code, 
  FileText, 
  HelpCircle, 
  Zap,
  Eye
} from 'lucide-react';

const navigation = [
  { name: 'Overview', href: '/', icon: Home },
  { name: 'Download', href: '/download', icon: Download },
  { name: 'Usage', href: '/usage', icon: Terminal },
  { name: 'API Reference', href: '/api', icon: Code },
  { name: 'Examples', href: '/examples', icon: FileText },
  { name: 'Troubleshooting', href: '/troubleshooting', icon: HelpCircle },
  { name: 'Future Plans', href: '/future', icon: Zap },
  { name: 'Transparency', href: '/transparency', icon: Eye },
];

const Sidebar: React.FC = () => {
  return (
    <aside className="w-64 bg-white dark:bg-gray-900 min-h-screen">
      <nav className="px-4 pt-20 pb-4 space-y-2">
        {navigation.map((item) => {
          const Icon = item.icon;
          return (
            <NavLink
              key={item.name}
              to={item.href}
              className={({ isActive }) =>
                `flex items-center space-x-3 px-3 py-2 rounded-lg text-sm font-medium transition-colors ${
                  isActive
                    ? 'bg-primary-100 dark:bg-primary-900 text-primary-700 dark:text-primary-300'
                    : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
                }`
              }
            >
              <Icon className="w-4 h-4" />
              <span>{item.name}</span>
            </NavLink>
          );
        })}
      </nav>
    </aside>
  );
};

export default Sidebar;
