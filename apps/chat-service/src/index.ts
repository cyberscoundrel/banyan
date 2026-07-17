import { loadConfig, Config } from './config.js';
import { createApp } from './server.js';
import { loadSigningKey } from './ledger/signing.js';

async function main() {
  const config = loadConfig();
  
  console.log('Starting Banyan Chat Service...');
  console.log(`Node ID: ${config.nodeId || 'not set'}`);
  console.log(`Listen: ${config.listenAddr}`);
  console.log(`Data dir: ${config.dataDir}`);
  console.log(`Keys: static=${config.hasStaticKey}, ledger=${config.hasLedgerKey}, mod=${config.hasModKey}, admin=${config.hasAdminKey}`);

  if (config.serviceKeyPem) {
    try {
      const { publicKeyHex } = loadSigningKey(config.serviceKeyPem);
      console.log(`Loaded service key: ${publicKeyHex.slice(0, 16)}...`);
    } catch (err) {
      console.error(`Failed to load service key: ${err}`);
    }
  }

  if (config.proxyUrl && config.figAlias) {
    console.log(`Sync via proxy: ${config.proxyUrl} (fig alias: ${config.figAlias})`);
  }

  const { server, hub, storage } = createApp(config);

  const [host, port] = parseAddr(config.listenAddr);
  
  server.listen(parseInt(port), host, () => {
    console.log(`Server listening on ${config.listenAddr}`);
  });

  const shutdown = () => {
    console.log('\nShutting down...');
    hub.close();
    storage.close();
    server.close(() => {
      console.log('Server closed');
      process.exit(0);
    });
    setTimeout(() => process.exit(1), 5000);
  };

  process.on('SIGINT', shutdown);
  process.on('SIGTERM', shutdown);
}

function parseAddr(addr: string): [string, string] {
  if (addr.startsWith(':')) {
    return ['0.0.0.0', addr.slice(1)];
  }
  const parts = addr.split(':');
  if (parts.length === 2) {
    return [parts[0], parts[1]];
  }
  return ['0.0.0.0', '8080'];
}

main().catch((err) => {
  console.error('Fatal error:', err);
  process.exit(1);
});
