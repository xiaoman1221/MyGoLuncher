import { mkdirSync, writeFileSync } from 'node:fs';
import { dirname } from 'node:path';

const key = (process.env.MGL_PRODUCT_API_KEY ?? '').trim();
const server = (process.env.MGL_PRODUCT_SERVER_URL ?? '').trim();
if (!key || !server) {
  console.error('MGL_PRODUCT_API_KEY and MGL_PRODUCT_SERVER_URL are required for an official build');
  process.exit(2);
}
let parsed;
try {
  parsed = new URL(server);
} catch {
  console.error('MGL_PRODUCT_SERVER_URL is not a valid URL');
  process.exit(2);
}
if (parsed.protocol !== 'https:' && !['localhost', '127.0.0.1'].includes(parsed.hostname)) {
  console.error('MGL_PRODUCT_SERVER_URL must use HTTPS (localhost is allowed for development)');
  process.exit(2);
}
const escape = (value) => String(value).replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\r/g, '\\r').replace(/\n/g, '\\n');
const name = escape((process.env.MGL_PRODUCT_NAME ?? '').trim() || 'MyGoLuncher');
const slug = escape((process.env.MGL_PRODUCT_SLUG ?? '').trim() || 'MyGo');
const content = `//go:build mgl_private

package license

func init() {
	ProductAPIKey = "${escape(key)}"
	ProductServerURL = "${escape(server)}"
	ProductName = "${name}"
	ProductSlug = "${slug}"
}
`;
const target = 'internal/license/product_private.go';
mkdirSync(dirname(target), { recursive: true });
writeFileSync(target, content, { encoding: 'utf8' });
console.log(`wrote ${target} for official build`);
