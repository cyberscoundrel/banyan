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
  syncEnabled: boolean;
  serviceKeyPem: string;
  proxyUrl: string;
  figAlias: string;
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

  return {
    listenAddr: process.env.LISTEN_ADDR || fileConfig.listenAddr || ':8080',
    dataDir: process.env.DATA_DIR || fileConfig.dataDir || './data',
    nodeId: process.env.NODE_ID || fileConfig.nodeId || '',
    hasStaticKey: getBool('HAS_STATIC_KEY', fileConfig.hasStaticKey ?? true),
    hasLedgerKey: getBool('HAS_LEDGER_KEY', fileConfig.hasLedgerKey ?? false),
    hasModKey: getBool('HAS_MOD_KEY', fileConfig.hasModKey ?? false),
    hasAdminKey: getBool('HAS_ADMIN_KEY', fileConfig.hasAdminKey ?? false),
    syncEnabled: getBool('SYNC_ENABLED', fileConfig.syncEnabled ?? true),
    serviceKeyPem: process.env.SERVICE_KEY_PEM || fileConfig.serviceKeyPem || '',
    proxyUrl: process.env.PROXY_URL || fileConfig.proxyUrl || 'http://127.0.0.1:9090',
    figAlias: process.env.FIG_ALIAS || fileConfig.figAlias || '',
  };
}
