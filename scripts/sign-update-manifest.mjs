import { readFileSync, writeFileSync } from 'node:fs';
import { createPrivateKey, sign } from 'node:crypto';

const [manifestPath, keyPath = '.signing/ota-private.pem'] = process.argv.slice(2);
if (!manifestPath) {
  console.error('usage: node scripts/sign-update-manifest.mjs <manifest.json> [private-key.pem]');
  process.exit(2);
}

const payload = readFileSync(manifestPath);
const key = createPrivateKey(readFileSync(keyPath));
const signature = sign(null, payload, key);
writeFileSync(manifestPath + '.sig', Buffer.from(signature).toString('base64'));
console.log('signed ' + manifestPath + ' -> ' + manifestPath + '.sig');
