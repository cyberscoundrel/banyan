import React from 'react';

const Future: React.FC = () => {
  return (
    <div className="prose">
      <h1>Future of the Project</h1>

      <h2>Ethereum Integration & Service Alias System</h2>
      <p>
        The LibP2P HTTP Proxy Node is evolving to include Ethereum blockchain integration 
        for enhanced service discovery and micropayment capabilities. This integration will provide:
      </p>

      <h3>Service Alias System</h3>
      <ul>
        <li><strong>Blockchain-Based Aliases</strong>: Register human-readable service aliases on Ethereum</li>
        <li><strong>Decentralized Registry</strong>: Service aliases stored in smart contracts, ensuring global uniqueness and ownership</li>
        <li><strong>DNS-Style Resolution</strong>: Resolve service aliases to cryptographic service keys and peer connections</li>
        <li><strong>Ownership Transfer</strong>: Transfer alias ownership between Ethereum addresses</li>
        <li><strong>Registration Fees</strong>: Alias registration will require payment in ETH or tokens to prevent spam and fund development</li>
      </ul>

      <h3>Micropayment Endpoints</h3>
      <ul>
        <li><strong>Pay-Per-Use Services</strong>: Enable services to charge for API calls, data access, or computational resources</li>
        <li><strong>Automatic Billing</strong>: Smart contracts handle automatic billing and payment collection</li>
        <li><strong>Usage Metering</strong>: Track service usage and generate billing based on consumption</li>
        <li><strong>Multi-Token Support</strong>: Accept payments in ETH, stablecoins, or custom tokens</li>
        <li><strong>Escrow Services</strong>: Hold payments in escrow until service delivery is confirmed</li>
      </ul>

      <h3>Smart Contract Taxation</h3>
      <ul>
        <li><strong>Development Fund</strong>: A percentage of micropayment transactions will be allocated to a development fund</li>
        <li><strong>LibP2P Ecosystem Support</strong>: Portion of proceeds will support the broader LibP2P ecosystem and its maintainers</li>
        <li><strong>Transparent Allocation</strong>: Smart contracts will contribute funds to:</li>
        <ul>
          <li>Core development team</li>
          <li>LibP2P project maintenance</li>
          <li>Infrastructure costs (bootstrap nodes, documentation, etc.)</li>
          <li>Community grants for ecosystem development</li>
        </ul>
      </ul>

      <h2>Blockchain-Optional Architecture</h2>

      <h3>Dual Distribution Model</h3>
      <p>The project will offer two distribution variants:</p>

      <ol>
        <li><strong>Full Version</strong>: Includes Ethereum integration, micropayments, and service alias system</li>
        <li><strong>Blockchain-Free Version</strong>: Compiled without any blockchain dependencies for users who prefer:</li>
        <ul>
          <li>No cryptocurrency requirements</li>
          <li>Reduced binary size and dependencies</li>
          <li>Air-gapped or restricted network environments</li>
          <li>Pure peer-to-peer functionality without external dependencies</li>
        </ul>
      </ol>

      <h3>Feature Compatibility</h3>
      <p>Both versions will maintain full compatibility for:</p>
      <ul>
        <li>HTTP proxy functionality</li>
        <li>Peer discovery and connection management</li>
        <li>Cryptographic service discovery (using public keys)</li>
        <li>Real-time event monitoring</li>
        <li>Management API</li>
      </ul>

      <p>The blockchain-free version will simply omit:</p>
      <ul>
        <li>Ethereum service alias resolution</li>
        <li>Micropayment processing</li>
        <li>Smart contract interactions</li>
      </ul>
    </div>
  );
};

export default Future;

