import React from 'react';
import { BrowserRouter as Router, Routes, Route } from 'react-router-dom';
import Layout from './components/Layout';
import Home from './pages/Home';
import Usage from './pages/Usage';
import API from './pages/API';
import Examples from './pages/Examples';
import Download from './pages/Download';
import Docs from './pages/Docs';
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
            <Route path="/download" element={<Download />} />
            <Route path="/docs/*" element={<Docs />} />
          </Routes>
        </Layout>
      </Router>
    </ThemeProvider>
  );
}

export default App;
