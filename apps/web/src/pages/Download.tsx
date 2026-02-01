import React, { useState, useEffect } from 'react';
import { Download as DownloadIcon, ExternalLink, Shield, AlertTriangle, ChevronDown, Package, Check } from 'lucide-react';

interface Deliverable {
  id: string;
  platform: string;
  arch: string;
  filename: string;
  size: string;
  sizeBytes: number;
  checksum: string;
  contents: string[];
}

interface ReleaseMetadata {
  version: string;
  commit: string;
  buildDate: string;
  buildTime: string;
  whatsNew: string;
  deliverables?: {
    windows_x64: Deliverable;
    linux_x64: Deliverable;
    macos_x64: Deliverable;
    macos_arm64: Deliverable;
  };
}

// Use local dist in development, production URL otherwise
const getBaseUrl = (): string => {
  if (process.env.NODE_ENV === 'development') {
    return '/releases';
  }
  return 'https://releases.banyan.cyberscoundrel.com';
};

const Download: React.FC = () => {
  const [metadata, setMetadata] = useState<ReleaseMetadata | null>(null);
  const [loading, setLoading] = useState(true);
  const [selectedPlatform, setSelectedPlatform] = useState<string>('');
  const [dropdownOpen, setDropdownOpen] = useState(false);

  const baseUrl = getBaseUrl();

  useEffect(() => {
    const fetchMetadata = async () => {
      try {
        const response = await fetch(`${baseUrl}/release-metadata.json`);
        if (response.ok) {
          const data = await response.json();
          setMetadata(data);
          
          // Auto-detect platform
          const detectedPlatform = detectPlatform();
          if (data.deliverables?.[detectedPlatform]) {
            setSelectedPlatform(detectedPlatform);
          } else if (data.deliverables) {
            setSelectedPlatform(Object.keys(data.deliverables)[0]);
          }
        }
      } catch (error) {
        console.error('Failed to fetch release metadata:', error);
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
  }, [baseUrl]);

  // Detect user's platform
  const detectPlatform = (): string => {
    const userAgent = navigator.userAgent.toLowerCase();
    const platform = navigator.platform.toLowerCase();
    
    if (userAgent.includes('win')) {
      return 'windows_x64';
    } else if (userAgent.includes('mac')) {
      // Check for Apple Silicon
      if (platform.includes('arm') || (userAgent.includes('mac') && !userAgent.includes('intel'))) {
        return 'macos_arm64';
      }
      return 'macos_x64';
    } else if (userAgent.includes('linux')) {
      return 'linux_x64';
    }
    return 'windows_x64';
  };

  // Default fallback deliverables in case metadata is missing or empty
  const defaultDeliverables: Deliverable[] = [
    {
      id: 'windows_x64',
      platform: 'Windows',
      arch: 'x64',
      filename: 'banyan-windows-x64.zip',
      size: '25 MB',
      sizeBytes: 26214400,
      checksum: 'sha256:pending...',
      contents: ['banyan.exe', 'banyan-cli.exe', 'README.txt'],
    },
    {
      id: 'linux_x64',
      platform: 'Linux',
      arch: 'x64',
      filename: 'banyan-linux-x64.tar.gz',
      size: '24 MB',
      sizeBytes: 25165824,
      checksum: 'sha256:pending...',
      contents: ['banyan', 'banyan-cli', 'README.md'],
    },
    {
      id: 'macos_x64',
      platform: 'macOS',
      arch: 'x64 (Intel)',
      filename: 'banyan-macos-x64.tar.gz',
      size: '24 MB',
      sizeBytes: 25165824,
      checksum: 'sha256:pending...',
      contents: ['banyan', 'banyan-cli', 'README.md'],
    },
    {
      id: 'macos_arm64',
      platform: 'macOS',
      arch: 'ARM64 (Apple Silicon)',
      filename: 'banyan-macos-arm64.tar.gz',
      size: '23 MB',
      sizeBytes: 24117248,
      checksum: 'sha256:pending...',
      contents: ['banyan', 'banyan-cli', 'README.md'],
    },
  ];

  // Use deliverables from metadata if available and non-empty, otherwise use defaults
  const metadataDeliverables = metadata?.deliverables
    ? Object.values(metadata.deliverables)
    : [];
  const deliverables: Deliverable[] = metadataDeliverables.length > 0
    ? metadataDeliverables
    : defaultDeliverables;

  // Safely select deliverable with guaranteed fallback
  const selectedDeliverable = deliverables.find(d => d.id === selectedPlatform) || deliverables[0];

  const getPlatformIcon = (platform: string): string => {
    switch (platform.toLowerCase()) {
      case 'windows': return '🪟';
      case 'linux': return '🐧';
      case 'macos': return '🍎';
      default: return '💻';
    }
  };

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

      <h2>Download</h2>
      
      <div className="not-prose">
        {/* Platform Selector */}
        <div className="mb-6">
          <label className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-2">
            Select your platform
          </label>
          <div className="relative">
            <button
              onClick={() => setDropdownOpen(!dropdownOpen)}
              className="w-full md:w-80 flex items-center justify-between px-4 py-3 bg-white dark:bg-gray-800 border border-gray-300 dark:border-gray-600 rounded-lg shadow-sm hover:border-primary-500 dark:hover:border-primary-400 focus:outline-none focus:ring-2 focus:ring-primary-500"
            >
              <div className="flex items-center space-x-3">
                <span className="text-xl">{getPlatformIcon(selectedDeliverable.platform)}</span>
                <div className="text-left">
                  <div className="font-medium text-gray-900 dark:text-white">
                    {selectedDeliverable.platform}
                  </div>
                  <div className="text-sm text-gray-500 dark:text-gray-400">
                    {selectedDeliverable.arch}
                  </div>
                </div>
              </div>
              <ChevronDown className={`w-5 h-5 text-gray-400 transition-transform ${dropdownOpen ? 'rotate-180' : ''}`} />
            </button>

            {dropdownOpen && (
              <div className="absolute z-10 mt-1 w-full md:w-80 bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg shadow-lg">
                {deliverables.map((deliverable) => (
                  <button
                    key={deliverable.id}
                    onClick={() => {
                      setSelectedPlatform(deliverable.id);
                      setDropdownOpen(false);
                    }}
                    className="w-full flex items-center justify-between px-4 py-3 hover:bg-gray-50 dark:hover:bg-gray-700 first:rounded-t-lg last:rounded-b-lg"
                  >
                    <div className="flex items-center space-x-3">
                      <span className="text-xl">{getPlatformIcon(deliverable.platform)}</span>
                      <div className="text-left">
                        <div className="font-medium text-gray-900 dark:text-white">
                          {deliverable.platform}
                        </div>
                        <div className="text-sm text-gray-500 dark:text-gray-400">
                          {deliverable.arch}
                        </div>
                      </div>
                    </div>
                    {selectedPlatform === deliverable.id && (
                      <Check className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                    )}
                  </button>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Download Card */}
        <div className="border border-gray-200 dark:border-gray-700 rounded-xl p-6 bg-gray-50 dark:bg-gray-800/50">
          <div className="flex items-start justify-between flex-wrap gap-4">
            <div className="flex-1 min-w-0">
              <div className="flex items-center space-x-2 mb-2">
                <Package className="w-5 h-5 text-primary-600 dark:text-primary-400" />
                <h3 className="text-lg font-semibold text-gray-900 dark:text-white m-0">
                  {selectedDeliverable.filename}
                </h3>
              </div>
              <p className="text-sm text-gray-600 dark:text-gray-400 mb-3">
                {selectedDeliverable.size} • {selectedDeliverable.platform} {selectedDeliverable.arch}
              </p>
              
              <div className="mb-3">
                <p className="text-xs font-medium text-gray-500 dark:text-gray-400 mb-1">Package contents:</p>
                <div className="flex flex-wrap gap-2">
                  {selectedDeliverable.contents.map((file, idx) => (
                    <span 
                      key={idx}
                      className="inline-flex items-center px-2 py-1 bg-gray-200 dark:bg-gray-700 rounded text-xs font-mono text-gray-700 dark:text-gray-300"
                    >
                      {file}
                    </span>
                  ))}
                </div>
              </div>

              <div className="flex items-center space-x-2 text-xs text-gray-500 dark:text-gray-500">
                <Shield className="w-3 h-3" />
                <span className="font-mono break-all">{selectedDeliverable.checksum}</span>
              </div>
            </div>

            <a
              href={`${baseUrl}/${selectedDeliverable.filename}`}
              download
              className="flex items-center space-x-2 bg-primary-600 hover:bg-primary-700 text-white px-6 py-3 rounded-lg font-medium shadow-sm hover:shadow-md transition-all"
            >
              <DownloadIcon className="w-5 h-5" />
              <span>Download</span>
            </a>
          </div>
        </div>
      </div>

      <h2>Installation Instructions</h2>

      <h3>Windows</h3>
      <ol>
        <li>Download and extract the ZIP file</li>
        <li>Open Command Prompt or PowerShell in the extracted folder</li>
        <li>Run <code>banyan-cli.exe</code> for the terminal UI, or <code>banyan.exe</code> for headless operation</li>
      </ol>

      <h3>macOS</h3>
      <ol>
        <li>Download and extract the tar.gz file: <code>tar -xzf banyan-macos-*.tar.gz</code></li>
        <li>Navigate to the extracted folder: <code>cd banyan</code></li>
        <li>Make the binaries executable: <code>chmod +x banyan banyan-cli</code></li>
        <li>Run <code>./banyan-cli</code> for the terminal UI</li>
      </ol>
      <p>
        <strong>Note:</strong> You may need to allow the binaries in System Preferences → Security & Privacy 
        if macOS blocks them due to being unsigned.
      </p>

      <h3>Linux</h3>
      <ol>
        <li>Download and extract the tar.gz file: <code>tar -xzf banyan-linux-x64.tar.gz</code></li>
        <li>Navigate to the extracted folder: <code>cd banyan</code></li>
        <li>Make the binaries executable: <code>chmod +x banyan banyan-cli</code></li>
        <li>Run <code>./banyan-cli</code> for the terminal UI</li>
      </ol>

      <h2>Verification</h2>
      <p>To verify the integrity of your download, check the SHA256 checksum:</p>

      <h3>Windows (PowerShell)</h3>
      <pre><code>Get-FileHash banyan-windows-x64.zip -Algorithm SHA256</code></pre>

      <h3>macOS/Linux</h3>
      <pre><code>sha256sum banyan-*.tar.gz</code></pre>

      <p>Compare the output with the checksum shown above for your platform.</p>

      <h2>System Requirements</h2>
      <ul>
        <li><strong>Memory:</strong> Minimum 512 MB RAM, recommended 1 GB+</li>
        <li><strong>Disk Space:</strong> 100 MB for binaries, additional space for logs and keys</li>
        <li><strong>Network:</strong> Internet connection for peer discovery (DHT)</li>
        <li><strong>Ports:</strong> Random high port for LibP2P (configurable)</li>
      </ul>

      <div className="flex items-center space-x-2 text-sm text-gray-600 dark:text-gray-400 mt-8">
        <ExternalLink className="w-4 h-4" />
        <span>Looking for source code? Visit the project repository on GitHub.</span>
      </div>
    </div>
  );
};

export default Download;
