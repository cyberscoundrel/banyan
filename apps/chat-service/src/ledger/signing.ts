import { createPublicKey, createPrivateKey } from 'crypto';
import { readFileSync, existsSync } from 'fs';
import { resolve } from 'path';
import nodeCrypto from 'crypto';

let privateKey: ReturnType<typeof createPrivateKey> | null = null;
let publicKeyHex: string | null = null;

export function loadSigningKey(pemPath: string): { publicKeyHex: string } {
  if (!existsSync(pemPath)) {
    throw new Error(`Service key PEM not found: ${pemPath}`);
  }

  const pem = readFileSync(resolve(pemPath), 'utf-8');
  privateKey = createPrivateKey({ key: pem, format: 'pem', type: 'pkcs8' });

  const pub = createPublicKey(privateKey);
  const pubDer = pub.export({ type: 'spki', format: 'der' });
  publicKeyHex = Buffer.from(pubDer).toString('hex');

  return { publicKeyHex };
}

export function getPublicKeyHex(): string {
  if (!publicKeyHex) {
    throw new Error('Signing key not loaded');
  }
  return publicKeyHex;
}

export function signPayload(payload: string): { signature: string; publicKey: string } {
  if (!privateKey || !publicKeyHex) {
    throw new Error('Signing key not loaded');
  }

  const payloadBytes = Buffer.from(payload, 'utf-8');
  const signature = nodeCrypto.sign(null, payloadBytes, privateKey);
  const sigHex = signature.toString('hex');

  return { signature: sigHex, publicKey: publicKeyHex };
}

export function verifySignature(payload: string, signatureHex: string, pubKeyHex: string): boolean {
  try {
    const pubDer = Buffer.from(pubKeyHex, 'hex');
    console.error(`[VERIFY] keyLen=${pubDer.length} keyStart=${pubDer.toString('hex', 0, 20)} payloadLen=${payload.length} sigLen=${Buffer.from(signatureHex, 'hex').length}`);
    const pub = createPublicKey({ key: pubDer, format: 'der', type: 'spki' });
    const pubRaw = pub.export({ type: 'spki', format: 'der' });

    const payloadBytes = Buffer.from(payload, 'utf-8');
    const sigBuf = Buffer.from(signatureHex, 'hex');

    const result = nodeCrypto.verify(null, payloadBytes, sigBuf, pubRaw);
    if (!result) {
      console.error(`[VERIFY] Verification returned false`);
    }
    return result;
  } catch (e) {
    console.error(`[VERIFY] Error: ${e}`);
    return false;
  }
}
