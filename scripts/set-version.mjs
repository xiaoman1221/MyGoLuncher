import { readFileSync, writeFileSync } from 'node:fs';

const version = (process.argv[2] ?? '').replace(/^v/, '').trim();
if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
  console.error('usage: node scripts/set-version.mjs <x.y.z>');
  process.exit(2);
}
const read = (path) => readFileSync(path, 'utf8');
const write = (path, value) => writeFileSync(path, value, 'utf8');
const updateJSON = (path, callback) => {
  const value = JSON.parse(read(path));
  callback(value);
  write(path, JSON.stringify(value, null, 2) + '\n');
};
const replaceOne = (path, pattern, replacement) => {
  const before = read(path);
  const after = before.replace(pattern, replacement);
  if (after === before) throw new Error(`could not update ${path}`);
  write(path, after);
};

updateJSON('wails.json', (value) => { value.info.productVersion = version; });
updateJSON('frontend/package.json', (value) => { value.version = version; });
updateJSON('frontend/package-lock.json', (value) => {
  value.version = version;
  if (value.packages?.['']) value.packages[''].version = version;
});
replaceOne('app.go', /const appVersion = "[^"]+"/, `const appVersion = "${version}"`);
replaceOne('internal/license/client.go', /var AppVersion = "[^"]+"/, `var AppVersion = "${version}"`);
replaceOne('internal/license/manager.go', /version = "[^"]+"/, `version = "${version}"`);
replaceOne('internal/launcher/launcher.go', /"launcher_version":\s+"[^"]+"/, `"launcher_version":      "${version}"`);
replaceOne('internal/launcher/launcher.go', /-Dminecraft\.launcher\.version=[^\s"]+/, `-Dminecraft.launcher.version=${version}`);
console.log(`version updated to ${version}`);
