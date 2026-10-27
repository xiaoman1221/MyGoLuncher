import { readFileSync } from 'node:fs';

const raw = process.argv[2] ?? '';
const expected = raw.replace(/^v/, '').trim();
if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(expected)) {
  console.error(`invalid release version: ${raw}`);
  process.exit(2);
}
const failures = [];
const wails = JSON.parse(readFileSync('wails.json', 'utf8'));
if (wails?.info?.productVersion !== expected) failures.push(`wails.json productVersion=${wails?.info?.productVersion}`);
const pkg = JSON.parse(readFileSync('frontend/package.json', 'utf8'));
if (pkg.version !== expected) failures.push(`frontend/package.json version=${pkg.version}`);
const lock = JSON.parse(readFileSync('frontend/package-lock.json', 'utf8'));
if (lock.version !== expected) failures.push(`frontend/package-lock.json version=${lock.version}`);
if (lock?.packages?.['']?.version !== expected) failures.push(`frontend/package-lock.json packages[""].version=${lock?.packages?.['']?.version}`);
const appGo = readFileSync('internal/app/app.go', 'utf8');
const appVersion = appGo.match(/const appVersion = "([^"]+)"/)?.[1];
if (appVersion !== expected) failures.push(`app.go appVersion=${appVersion}`);
const clientGo = readFileSync('internal/license/client.go', 'utf8');
const clientVersion = clientGo.match(/var AppVersion = "([^"]+)"/)?.[1];
if (clientVersion !== expected) failures.push(`internal/license/client.go AppVersion=${clientVersion}`);
const managerGo = readFileSync('internal/license/manager.go', 'utf8');
const managerVersion = managerGo.match(/version = "([^"]+)"/)?.[1];
if (managerVersion !== expected) failures.push(`internal/license/manager.go version=${managerVersion}`);
const launcherGo = readFileSync('internal/launcher/launcher.go', 'utf8');
const launcherVersion = launcherGo.match(/"launcher_version":\s+"([^"]+)"/)?.[1];
if (launcherVersion !== expected) failures.push(`internal/launcher/launcher.go launcher_version=${launcherVersion}`);
const launcherJvm = launcherGo.match(/-Dminecraft\.launcher\.version=([^\s"]+)/)?.[1];
if (launcherJvm !== expected) failures.push(`internal/launcher/launcher.go -Dminecraft.launcher.version=${launcherJvm}`);
if (failures.length) {
  console.error('release version mismatch:');
  for (const failure of failures) console.error('  - ' + failure);
  console.error(`set all version fields to ${expected} before tagging`);
  process.exit(1);
}
console.log(`release version ${expected} is consistent`);
