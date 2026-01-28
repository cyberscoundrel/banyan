import React from 'react';
import { useLocation } from 'react-router-dom';
import Sidebar from './Sidebar';
import DocsSidebar from './DocsSidebar';
import Header from './Header';

interface LayoutProps {
  children: React.ReactNode;
}

const Layout: React.FC<LayoutProps> = ({ children }) => {
  const location = useLocation();
  const isDocsRoute = location.pathname.startsWith('/docs');

  return (
    <div className="min-h-screen bg-white dark:bg-gray-900">
      <div className="fixed top-0 left-0 right-0 z-20">
        <Header />
      </div>
      <div className="flex pt-16">
        {isDocsRoute ? (
          <div className="fixed top-16 left-0 z-10 h-[calc(100vh-4rem)] overflow-y-auto docs-sidebar-scroll">
            <DocsSidebar />
          </div>
        ) : (
          <div className="fixed top-0 left-0 z-10">
            <Sidebar />
          </div>
        )}
        <main className="flex-1 ml-64 p-8 max-w-4xl mx-auto w-full">
          {children}
        </main>
      </div>
    </div>
  );
};

export default Layout;
