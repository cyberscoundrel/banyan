import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Home from './pages/Home';

import Usage from './pages/Usage';
import API from './pages/API';
import Examples from './pages/Examples';
import Troubleshooting from './pages/Troubleshooting';
import Future from './pages/Future';
import Download from './pages/Download';
import Transparency from './pages/Transparency';
import { ThemeProvider } from './contexts/ThemeContext';

function App() {
  return (
    <ThemeProvider>
      <Router>
        <Layout>
          <Routes>
            <Route path="/" element={<Home />} />
            <Route path="/usage" element={<Usage />} />
            <Route path="/api" element={<API />} />
            <Route path="/examples" element={<Examples />} />
            <Route path="/troubleshooting" element={<Troubleshooting />} />
            <Route path="/future" element={<Future />} />
            <Route path="/download" element={<Download />} />
            <Route path="/transparency" element={<Transparency />} />
          </Routes>
        </Layout>
      </Router>
    </ThemeProvider>
  );
}

export default App;
