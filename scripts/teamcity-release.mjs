#!/usr/bin/env node
// Cross-platform commercial release builder used by TeamCity.
// Builds the official (product key injected) client, optionally packs it with
// UPX, signs it with the platform tool, and publishes to releases/<version>/.

import { spawnSync } from 'node:child_process';
import { createHash } from 'node:crypto';
import {
  chmodSync, copyFileSync, existsSync, mkdirSync, readFileSync, rmSync, statSync, writeFileSync,
} from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const isWindows = process.platform === 'win32';
const args = process.argv.slice(2);

function argValue(name, fallback = '') {
  const index = args.indexOf(name);
  return index >= 0 && args[index + 1] ? args[index + 1] : fallback;
}

const platform = argValue('--platform', isWindows ? 'windows' : process.platform === 'darwin' ? 'macos' : 'linux');
const branch = argValue('--branch', process.env.TEAMCITY_BUILD_BRANCH ?? '');
const buildNumber = argValue('--build-number', process.env.BUILD_NUMBER ?? 'local');
const noPublish = args.includes('--no-publish');

if (!['windows', 'macos', 'linux'].includes(platform)) {
  fail(`不支持的平台: ${platform}`);
}
if (platform === 'windows' && !isWindows) fail('windows 构建必须在 Windows 代理上运行');
if (platform === 'macos' && process.platform !== 'darwin') fail('macos 构建必须在 macOS 代理上运行');
if (platform === 'linux' && process.platform !== 'linux') fail('linux 构建必须在 Linux 代理上运行');

const env = process.env;
const wails = JSON.parse(readFileSync(join(root, 'wails.json'), 'utf8'));
const version = String(wails?.info?.productVersion ?? '').trim();
if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`版本号无效: ${version}`);
}

function fail(message) {
  console.error(`ERROR: ${message}`);
  process.exit(1);
}

function command(name) {
  if (isWindows && name === 'npm') return 'npm.cmd';
  return name;
}

function run(name, argv, options = {}) {
  const executable = command(name);
  // Node cannot spawn .cmd files directly on Windows; route them through cmd.exe
  // instead of using shell:true (which concatenates arguments).
  const useCmd = isWindows && executable.toLowerCase().endsWith('.cmd');
  const result = spawnSync(
    useCmd ? (process.env.ComSpec ?? 'cmd.exe') : executable,
    useCmd ? ['/d', '/s', '/c', executable, ...argv] : argv,
    {
      cwd: options.cwd ?? root,
      stdio: options.quiet ? 'pipe' : 'inherit',
      encoding: 'utf8',
      env: { ...env, ...(options.env ?? {}) },
    },
  );
  if (result.error) throw new Error(`${name} 无法执行: ${result.error.message}`);
  if (result.status !== 0) {
    const detail = options.quiet ? `${result.stdout ?? ''}${result.stderr ?? ''}`.trim() : '';
    throw new Error(`${name} 退出码 ${result.status}${detail ? `: ${detail.slice(0, 800)}` : ''}`);
  }
  return result;
}

function capture(name, argv) {
  const result = spawnSync(command(name), argv, { cwd: root, encoding: 'utf8' });
  if (result.status !== 0) return '';
  return (result.stdout ?? '').trim();
}

function hasCommand(name) {
  return capture(isWindows ? 'where' : 'which', [name]).length > 0;
}

function hashFile(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

function writeJSON(path, value) {
  writeFileSync(path, `${JSON.stringify(value, null, 2)}\n`, 'utf8');
}

function resolveUpx() {
  if (env.MGL_UPX_PATH) {
    if (!existsSync(env.MGL_UPX_PATH)) fail(`env.MGL_UPX_PATH 指向的文件不存在: ${env.MGL_UPX_PATH}`);
    return env.MGL_UPX_PATH;
  }
  if (hasCommand('upx')) return 'upx';
  const local = [
    join(root, '.tools', 'upx', 'upx.exe'),
    join(root, '.tools', 'upx', 'upx'),
  ].find((path) => existsSync(path));
  if (local) return local;
  return '';
}

function packExecutable(path, backup = false) {
  const upx = resolveUpx();
  if (!upx) fail('未找到 UPX。请设置 env.MGL_UPX_PATH，或在代理 PATH / .tools/upx 中提供 upx');
  const packArgs = ['--best', '--lzma'];
  if (backup) packArgs.push('--backup');
  packArgs.push(path);
  run(upx, packArgs);
  run(upx, ['-t', path]);
}

function zipMacApp(appDir, target) {
  run('ditto', ['-c', '-k', '--keepParent', appDir, target]);
}

function archiveLinux(binary, target) {
  const stage = join(root, 'build', 'release-stage', 'linux');
  rmSync(stage, { recursive: true, force: true });
  mkdirSync(stage, { recursive: true });
  const stagedBinary = join(stage, 'MyMinecraftGoLuncher');
  copyFileSync(binary, stagedBinary);
  chmodSync(stagedBinary, 0o755);
  for (const name of ['LICENSE', 'NOTICE', 'README.md']) {
    const source = join(root, name);
    if (existsSync(source)) copyFileSync(source, join(stage, name));
  }
  run('tar', ['-czf', target, '-C', stage, '.']);
}

function architectureName() {
  if (process.arch === 'x64') return 'amd64';
  if (process.arch === 'arm64') return 'arm64';
  return process.arch;
}

function pushWithRetry(remote, token, user) {
  if (!/^https?:\/\//.test(remote)) {
    run('git', ['push', 'origin', 'HEAD:master']);
    return;
  }
  const url = new URL(remote);
  url.username = user;
  url.password = token;
  const pushUrl = url.toString();
  for (let attempt = 1; attempt <= 3; attempt += 1) {
    const pushed = spawnSync('git', ['push', pushUrl, 'HEAD:master'], {
      cwd: root, stdio: 'inherit', encoding: 'utf8',
    });
    if (pushed.status === 0) return;
    if (attempt === 3) fail('推送 releases/ 失败');
    // Another platform build may have committed first. Rebase our
    // platform-specific files onto the new master and retry.
    run('git', ['-c', 'rebase.autoStash=true', 'pull', '--rebase', pushUrl, 'refs/heads/master']);
  }
}

// ---------------------------------------------------------------------------

let branchName = branch.trim();
if (branchName === '<default>' || branchName === 'default') branchName = 'master';
if (branchName.includes('%')) branchName = '';
if (!branchName) branchName = capture('git', ['rev-parse', '--abbrev-ref', 'HEAD']).trim();
if (!branchName || branchName === 'HEAD') branchName = 'unknown';

const productKey = (env.MGL_PRODUCT_API_KEY ?? '').trim();
const productServer = (env.MGL_PRODUCT_SERVER_URL ?? '').trim();
const missing = [];
if (!productKey) missing.push('env.MGL_PRODUCT_API_KEY');
if (!productServer) missing.push('env.MGL_PRODUCT_SERVER_URL');
if (!noPublish && !(env.GIT_PUSH_TOKEN ?? '').trim()) missing.push('env.GIT_PUSH_TOKEN');
if (missing.length > 0) fail(`商用发布缺少必要参数: ${missing.join(', ')}`);
if (!/^https:\/\//i.test(productServer)) fail('商用构建要求 HTTPS 授权服务地址');

const upxEnabled = platform === 'windows' ? true : String(env.MGL_UPX ?? '').toLowerCase() === 'true';
if (upxEnabled && !resolveUpx()) {
  fail('未找到 UPX。请设置 env.MGL_UPX_PATH，或在代理 PATH / .tools/upx 中提供 upx');
}

let pfxPath = '';
if (platform === 'windows') {
  pfxPath = (env.MGL_PFX_PATH ?? '').trim();
  if (!pfxPath) fail('Windows 商用构建缺少 env.MGL_PFX_PATH');
  if (!existsSync(pfxPath)) fail(`找不到签名证书: ${pfxPath}`);
  if (!(env.MGL_SIGN_PFX_PASSWORD ?? '').trim()) fail('Windows 商用构建缺少 env.MGL_SIGN_PFX_PASSWORD');
}
const macIdentity = (env.MGL_MACOS_SIGN_IDENTITY ?? '').trim();
if (platform === 'macos' && !macIdentity) {
  console.warn('WARNING: 未设置 env.MGL_MACOS_SIGN_IDENTITY，macOS 产物不会进行 codesign。');
}

run(process.execPath, ['scripts/check-release-version.mjs', version]);

// Keep the private product config out of the committed tree.
const privateConfig = join(root, 'internal', 'license', 'product_private.go');
let built = false;
try {
  run(process.execPath, ['scripts/write-product-config.mjs'], { env: env });
  run(command('npm'), ['ci', '--no-audit', '--no-fund'], { cwd: join(root, 'frontend') });

  if (!hasCommand('wails')) {
    run('go', ['install', 'github.com/wailsapp/wails/v2/cmd/wails@v2.17.0']);
    const goPath = capture('go', ['env', 'GOPATH']);
    if (goPath) {
      const pathKey = isWindows && Object.prototype.hasOwnProperty.call(env, 'Path') ? 'Path' : 'PATH';
      env[pathKey] = `${join(goPath, 'bin')}${isWindows ? ';' : ':'}${env[pathKey] ?? env.PATH ?? ''}`;
    }
  }

  const ldflags = '-s -w -X MyMinecraftGoLuncher/internal/security.Enforce=true';
  run('wails', ['build', '-trimpath', '-ldflags', ldflags, '-tags', 'mgl_private']);
  built = true;
} finally {
  rmSync(privateConfig, { force: true });
}

if (!built) fail('构建未完成');

const arch = architectureName();
const outDir = join(root, 'releases', version, platform);
mkdirSync(outDir, { recursive: true });

let artifactName = '';
let artifactPath = '';
let signed = false;
let signatureStatus = 'not-signed';
let packed = false;

if (platform === 'windows') {
  const exe = join(root, 'build', 'bin', 'MyMinecraftGoLuncher.exe');
  if (!existsSync(exe)) fail(`构建产物不存在: ${exe}`);
  if (upxEnabled) {
    packExecutable(exe, true);
    packed = true;
  }
  const signArgs = [
    '-NoProfile', '-ExecutionPolicy', 'Bypass',
    '-File', join(root, 'scripts', 'sign-windows.ps1'),
    '-Exe', exe,
    '-PfxPath', pfxPath,
    '-TimestampUrl', String(env.MGL_WINDOWS_TIMESTAMP_URL ?? '').trim(),
  ];
  if (String(env.MGL_WINDOWS_TRUST_SELF_SIGNED ?? 'true').toLowerCase() !== 'false') {
    signArgs.push('-TrustSelfSigned');
  }
  run('powershell.exe', signArgs);
  signed = true;
  signatureStatus = 'signed';
  artifactName = `MyGoLuncher-${version}-windows-${arch}.exe`;
  artifactPath = join(outDir, artifactName);
  copyFileSync(exe, artifactPath);
} else if (platform === 'macos') {
  const app = join(root, 'build', 'bin', 'MyMinecraftGoLuncher.app');
  if (!existsSync(app)) fail(`构建产物不存在: ${app}`);
  if (upxEnabled) {
    const binary = join(app, 'Contents', 'MacOS', 'MyMinecraftGoLuncher');
    if (!existsSync(binary)) fail(`macOS 可执行文件不存在: ${binary}`);
    packExecutable(binary);
    packed = true;
  }
  if (macIdentity) {
    const signArgs = ['--force', '--deep', '--options', 'runtime'];
    if (String(env.MGL_MACOS_TIMESTAMP ?? 'false').toLowerCase() !== 'false') signArgs.push('--timestamp');
    else signArgs.push('--timestamp=none');
    signArgs.push('--sign', macIdentity, app);
    run('codesign', signArgs);
    run('codesign', ['--verify', '--deep', '--strict', '--verbose=2', app]);
    signed = true;
    signatureStatus = 'signed';
  }
  artifactName = `MyGoLuncher-${version}-macos-${arch}.zip`;
  artifactPath = join(outDir, artifactName);
  zipMacApp(app, artifactPath);
} else {
  const binary = join(root, 'build', 'bin', 'MyMinecraftGoLuncher');
  if (!existsSync(binary)) fail(`构建产物不存在: ${binary}`);
  if (upxEnabled) {
    packExecutable(binary);
    packed = true;
  }
  artifactName = `MyGoLuncher-${version}-linux-${arch}.tar.gz`;
  artifactPath = join(outDir, artifactName);
  archiveLinux(binary, artifactPath);
}

const sha256 = hashFile(artifactPath);
const size = statSync(artifactPath).size;
writeFileSync(join(outDir, 'SHA256SUMS.txt'), `${sha256}  ${artifactName}\n`, 'ascii');

if (platform === 'linux' && (env.MGL_GPG_KEY_ID ?? '').trim()) {
  const checksums = join(outDir, 'SHA256SUMS.txt');
  run('gpg', [
    '--batch', '--yes', '--local-user', env.MGL_GPG_KEY_ID.trim(),
    '--armor', '--detach-sign', checksums,
  ]);
  signatureStatus = 'gpg-signed';
}

const commit = capture('git', ['rev-parse', 'HEAD']);
const builtAt = new Date().toISOString();
const productMode = 'official';
const buildInfo = {
  version,
  platform,
  arch,
  commit,
  builtAt,
  buildNumber,
  productMode,
  upxPacked: packed,
  signed,
  signatureStatus,
  file: artifactName,
  sha256,
  size,
};
writeJSON(join(outDir, 'build-info.json'), buildInfo);
writeJSON(join(root, 'releases', `latest-${platform}.json`), {
  version,
  platform,
  path: `releases/${version}/${platform}/${artifactName}`,
  sha256,
  size,
  builtAt,
  commit,
  signed,
  upxPacked: packed,
});

if (noPublish) {
  console.log(`已生成产物（--no-publish，未提交）: ${artifactPath}`);
  process.exit(0);
}
if (branchName && branchName !== 'master' && branchName !== 'refs/heads/master') {
  console.log(`当前分支不是 master，跳过提交: ${branchName}`);
  process.exit(0);
}
if (!(env.GIT_PUSH_TOKEN ?? '').trim()) {
  console.warn('未配置 env.GIT_PUSH_TOKEN，已生成产物但跳过提交。');
  process.exit(0);
}

run('git', ['add', '--', 'releases']);
const diff = spawnSync('git', ['diff', '--cached', '--quiet'], { cwd: root });
if (diff.status === 0) {
  console.log('releases/ 没有变化，无需提交。');
  process.exit(0);
}
run('git', [
  '-c', 'user.name=TeamCity',
  '-c', 'user.email=teamcity@localhost',
  'commit', '-m', `[teamcity release] v${version} ${platform} build ${buildNumber}`,
]);

const remote = (env.MGL_GIT_PUSH_URL ?? '').trim() || capture('git', ['remote', 'get-url', 'origin']);
if (!remote) fail('无法确定推送地址');
pushWithRetry(remote, env.GIT_PUSH_TOKEN.trim(), (env.GIT_PUSH_USER ?? '').trim() || 'teamcity');
console.log(`发布完成: releases/${version}/${platform}/${artifactName}`);
