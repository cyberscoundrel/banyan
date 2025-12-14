import React, { useState, useEffect } from 'react';
import { Download as DownloadIcon, ExternalLink, Shield, AlertTriangle } from 'lucide-react';

interface DownloadInfo {
  platform: string;
  arch: string;
  filename: string;
  size: string;
  sizeBytes: number;
  checksum: string;
}

interface ReleaseMetadata {
  version: string;
  commit: string;
  buildDate: string;
  buildTime: string;
  whatsNew: string;
  downloads?: {
    windows: DownloadInfo;
    linux: DownloadInfo;
    macos_intel: DownloadInfo;
    macos_arm: DownloadInfo;
  };
}

const Download: React.FC = () => {
  const [metadata, setMetadata] = useState<ReleaseMetadata | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchMetadata = async () => {
      try {
        const response = await fetch('https://releases.banyan.cyberscoundrel.com/release-metadata.json');
        if (response.ok) {
          const data = await response.json();
          setMetadata(data);
        }
      } catch (error) {
        console.error('Failed to fetch release metadata:', error);
        // Fallback metadata
        setMetadata({
          version: 'pre-release',
          commit: 'unknown',
          buildDate: 'unknown',
          buildTime: 'unknown',
          whatsNew: '😊'
        });
      } finally {
        setLoading(false);
      }
    };

    fetchMetadata();
  }, []);

  // Get downloads from metadata or fallback to hardcoded values
  const downloads = metadata?.downloads ? [
    metadata.downloads.windows,
    metadata.downloads.macos_intel,
    metadata.downloads.macos_arm,
    metadata.downloads.linux,
  ] : [
    {
      platform: 'Windows',
      arch: 'x64',
      filename: 'banyan.exe',
      size: '12.3 MB',
      sizeBytes: 12900000,
      checksum: 'sha256:a1b2c3d4e5f6...',
    },
    {
      platform: 'macOS',
      arch: 'x64 (Intel)',
      filename: 'banyan-amd64',
      size: '11.8 MB',
      sizeBytes: 12300000,
      checksum: 'sha256:f6e5d4c3b2a1...',
    },
    {
      platform: 'macOS',
      arch: 'ARM64 (Apple Silicon)',
      filename: 'banyan-arm64',
      size: '11.2 MB',
      sizeBytes: 11700000,
      checksum: 'sha256:b2a1c3d4e5f6...',
    },
    {
      platform: 'Linux',
      arch: 'x64',
      filename: 'banyan',
      size: '11.5 MB',
      sizeBytes: 12000000,
      checksum: 'sha256:c3d4e5f6a1b2...',
    },
  ];

  return (
    <div className="prose max-w-none">
      <h1>Download Banyan</h1>
      

      <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4 mb-6">
        <div className="flex items-start space-x-2">
          <AlertTriangle className="w-5 h-5 text-yellow-600 dark:text-yellow-400 mt-0.5 flex-shrink-0" />
          <div>
            <h3 className="text-sm font-medium text-yellow-800 dark:text-yellow-200 mt-0 mb-1">
              Prerelease Software
            </h3>
            <p className="text-sm text-yellow-700 dark:text-yellow-300 mb-0">
              These are development builds intended for evaluation and testing only. 
              Not recommended for production use.
            </p>
          </div>
        </div>
      </div>

      <h2>Latest Release: {loading ? 'Loading...' : metadata?.version || 'Unknown'}</h2>
      <p>
        <strong>Release Date:</strong> {loading ? 'Loading...' : (metadata?.buildDate ? new Date(metadata.buildDate).toLocaleDateString() : 'Unknown')}<br />
        <strong>Build:</strong> {loading ? 'Loading...' : (metadata?.commit ? `commit ${metadata.commit.substring(0, 8)}` : 'Unknown')}
      </p>

      <h3>What's New</h3>
      {loading ? (
        <p>Loading...</p>
      ) : metadata?.whatsNew && metadata.whatsNew !== '😊' ? (
        <ul>
          {metadata.whatsNew.split(',').map((item, index) => (
            <li key={index}>{item.trim()}</li>
          ))}
        </ul>
      ) : (
        <p>{metadata?.whatsNew || 'nothing 😊'}</p>
      )}

      <h2>Download Links</h2>
      <div className="grid gap-4 not-prose">
        {downloads.map((download, index) => (
          <div
            key={index}
            className="border border-gray-200 dark:border-gray-700 rounded-lg p-4 hover:border-primary-300 dark:hover:border-primary-600 transition-colors"
          >
            <div className="flex items-center justify-between">
              <div className="flex-1">
                <div className="flex items-center space-x-2 mb-1">
                  <h3 className="text-lg font-semibold text-gray-900 dark:text-white">
                    {download.platform}
                  </h3>
                  <span className="text-sm text-gray-500 dark:text-gray-400">
                    {download.arch}
                  </span>
                </div>
                <p className="text-sm text-gray-600 dark:text-gray-400 mb-2">
                  {download.filename} • {download.size}
                </p>
                <p className="text-xs text-gray-500 dark:text-gray-500 font-mono">
                  {download.checksum}
                </p>
              </div>
              <a 
                href={`https://releases.banyan.cyberscoundrel.com/${download.platform === 'macOS' ? 'osx' : download.platform === 'Windows' ? 'win' : download.platform.toLowerCase()}/${download.filename}`}
                download
                className="flex items-center space-x-2 bg-primary-600 hover:bg-primary-700 text-white px-4 py-2 rounded-lg transition-colors"
              >
                <DownloadIcon className="w-4 h-4" />
                <span>Download</span>
              </a>
            </div>
          </div>
        ))}
      </div>

      <h2>Installation Instructions</h2>

      <h3>Windows</h3>
      <ol>
        <li>Download the Windows executable</li>
        <li>Open Command Prompt or PowerShell</li>
        <li>Navigate to the download location</li>
        <li>Run: <code>banyan.exe</code></li>
      </ol>

      <h3>macOS</h3>
      <ol>
        <li>Download the appropriate macOS binary for your architecture</li>
        <li>Open Terminal</li>
        <li>Make the binary executable: <code>chmod +x banyan-*</code></li>
        <li>Run: <code>./banyan-amd64</code> (Intel) or <code>./banyan-arm64</code> (Apple Silicon)</li>
      </ol>
      <p>
        <strong>Note:</strong> You may need to allow the binary in System Preferences → Security & Privacy 
        if macOS blocks it due to being unsigned.
      </p>

      <h3>Linux</h3>
      <ol>
        <li>Download the Linux binary for your architecture</li>
        <li>Open a terminal</li>
        <li>Make the binary executable: <code>chmod +x banyan</code></li>
        <li>Run: <code>./banyan</code></li>
      </ol>

      <h2>Verification</h2>
      <p>To verify the integrity of your download, check the SHA256 checksum:</p>

      <h3>Windows (PowerShell)</h3>
      <pre><code>Get-FileHash banyan.exe -Algorithm SHA256</code></pre>
      {metadata?.downloads?.windows && (
        <p>Expected: <code>{metadata.downloads.windows.checksum}</code></p>
      )}

      <h3>macOS/Linux</h3>
      <pre><code>sha256sum banyan*</code></pre>
      {metadata?.downloads && (
        <div>
          <p><strong>Expected checksums:</strong></p>
          <ul>
            <li>Linux: <code>{metadata.downloads.linux.checksum}</code></li>
            <li>macOS Intel: <code>{metadata.downloads.macos_intel.checksum}</code></li>
            <li>macOS ARM: <code>{metadata.downloads.macos_arm.checksum}</code></li>
          </ul>
        </div>
      )}

      <p>Compare the output with the checksum listed above for your platform.</p>

      <h2>System Requirements</h2>
      <ul>
        <li><strong>Memory:</strong> Minimum 512 MB RAM, recommended 1 GB+</li>
        <li><strong>Disk Space:</strong> 50 MB for binary, additional space for logs and keys</li>
        <li><strong>Network:</strong> Internet connection for peer discovery (DHT)</li>
        <li><strong>Ports:</strong> Random high port for LibP2P (configurable)</li>
      </ul>

      <div className="flex items-center space-x-2 text-sm text-gray-600 dark:text-gray-400 mt-8">
        <ExternalLink className="w-4 h-4" />
        <span>Looking for source code? It will be available after the open source release.</span>
      </div>
    </div>
  );
};

export default Download;

