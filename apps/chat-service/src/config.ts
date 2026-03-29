import { readFileSync, existsSync } from 'fs';
import { resolve } from 'path';

export interface Config {
  listenAddr: string;
  dataDir: string;
  nodeId: string;
  hasStaticKey: boolean;
  hasLedgerKey: boolean;
  hasModKey: boolean;
  hasAdminKey: boolean;
  syncPeers: string[];
  syncEnabled: boolean;
  syncInterval: number;
}

function parseDuration(s: string): number {
  const match = s.match(/^(\d+)(ms|s|m|h)$/);
  if (!match) return parseInt(s) || 30000;
  const [, val, unit] = match;
  const ms = parseInt(val);
  switch (unit) {
    case 'ms': return ms;
    case 's': return ms * 1000;
    case 'm': return ms * 60000;
    case 'h': return ms * 3600000;
    default: return ms;
  }
}

export function loadConfig(): Config {
  const configPath = process.env.CONFIG_PATH || resolve('./config.json');
  
  let fileConfig: Partial<Config> = {};
  if (existsSync(configPath)) {
    const content = readFileSync(configPath, 'utf-8');
    fileConfig = JSON.parse(content);
  }

  const getBool = (key: string, def: boolean): boolean => {
    const env = process.env[key];
    if (env !== undefined) return env === 'true' || env === '1';
    return def;
  };

  const syncInterval = fileConfig.syncInterval 
    ? parseDuration(String(fileConfig.syncInterval))
    : 30000;

  return {
    listenAddr: process.env.LISTEN_ADDR || fileConfig.listenAddr || ':8080',
    dataDir: process.env.DATA_DIR || fileConfig.dataDir || './data',
    nodeId: process.env.NODE_ID || fileConfig.nodeId || '',
    hasStaticKey: getBool('HAS_STATIC_KEY', fileConfig.hasStaticKey ?? true),
    hasLedgerKey: getBool('HAS_LEDGER_KEY', fileConfig.hasLedgerKey ?? false),
    hasModKey: getBool('HAS_MOD_KEY', fileConfig.hasModKey ?? false),
    hasAdminKey: getBool('HAS_ADMIN_KEY', fileConfig.hasAdminKey ?? false),
    syncPeers: process.env.SYNC_PEERS 
      ? process.env.SYNC_PEERS.split(',').filter(Boolean)
      : fileConfig.syncPeers || [],
    syncEnabled: getBool('SYNC_ENABLED', fileConfig.syncEnabled ?? true),
    syncInterval,
  };
}
