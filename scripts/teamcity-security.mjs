#!/usr/bin/env node
// Security gate used by TeamCity release builds and by the standalone
// "Security Checks" build type.
//
// Checks:
//   1. Sensitive files are not tracked by git.
//   2. Tracked text files do not contain common credential patterns.
//   3. gitleaks (if installed) finds no leaked secrets.
//   4. go vet ./...
//   5. govulncheck ./...
//   6. npm audit for runtime frontend dependencies.

import { spawnSync } from 'node:child_process';
import { existsSync, lstatSync, readFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

const root = dirname(dirname(fileURLToPath(import.meta.url)));
const isWindows = process.platform === 'win32';
const args = process.argv.slice(2);
const quick = args.includes('--quick');
let failures = 0;

function heading(text) {
  console.log(`\n==> ${text}`);
}

function fail(message) {
  failures += 1;
  console.error(`FAIL: ${message}`);
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
      env: { ...process.env, ...(options.env ?? {}) },
    },
  );
  if (result.error) {
    fail(`${name} 无法执行: ${result.error.message}`);
    return 127;
  }
  if (result.status !== 0) {
    if (options.quiet) {
      const detail = `${result.stdout ?? ''}${result.stderr ?? ''}`.trim();
      if (detail) console.error(detail.slice(0, 1200));
    }
    return result.status ?? 1;
  }
  return 0;
}

function capture(name, argv) {
  const result = spawnSync(command(name), argv, { cwd: root, encoding: 'utf8' });
  return result.status === 0 ? (result.stdout ?? '').trim() : '';
}

function trackedFiles() {
  const result = spawnSync('git', ['ls-files', '-z'], { cwd: root });
  if (result.status !== 0) {
    fail('无法读取 git 跟踪文件列表');
    return [];
  }
  return result.stdout.toString('utf8').split('\0').filter(Boolean);
}

function hasCommand(name) {
  return capture(isWindows ? 'where' : 'which', [name]).length > 0;
}

// ---------------------------------------------------------------------------
// 1. Sensitive files must never be committed.

heading('敏感文件检查');
const forbiddenFiles = [
  {
    pattern: /^internal\/license\/product_private\.go$/,
    reason: '产品密钥构建文件不得进入仓库',
  },
  { pattern: /(^|\/)\.env(\..*)?$/, reason: '.env 文件不得进入仓库', allow: /\.env\.example$/ },
  { pattern: /\.(pfx|p12|pvk|snk)$/i, reason: '签名私钥/证书不得进入仓库' },
  { pattern: /(^|\/)secrets\.(dat|key)$/i, reason: '本地加密密钥不得进入仓库' },
  { pattern: /(^|\/)\.signing\//, reason: '本地签名目录不得进入仓库' },
  { pattern: /(^|\/)\.idea\//, reason: 'IDE 配置不得进入仓库' },
  { pattern: /(^|\/)\.tools\//, reason: '本地工具目录不得进入仓库' },
  { pattern: /(^|\/)MGL\//, reason: '便携运行数据不得进入仓库' },
  { pattern: /(^|\/)\.minecraft\//, reason: '游戏目录不得进入仓库' },
];
let sensitiveFound = false;
for (const file of trackedFiles()) {
  for (const rule of forbiddenFiles) {
    if (rule.allow?.test(file)) continue;
    if (rule.pattern.test(file)) {
      sensitiveFound = true;
      fail(`${file}: ${rule.reason}`);
      break;
    }
  }
}
if (!sensitiveFound) console.log('OK: 未发现被跟踪的敏感文件');

// ---------------------------------------------------------------------------
// 2. Built-in credential pattern scan.

heading('密钥与令牌扫描');
const secretPatterns = [
  { name: 'private key block', pattern: /-----BEGIN (?:RSA|EC|DSA|OPENSSH|PGP) PRIVATE KEY-----/ },
  { name: 'AWS access key', pattern: /\bAKIA[0-9A-Z]{16}\b/ },
  { name: 'GitHub token', pattern: /\b(?:ghp_[A-Za-z0-9]{36}|github_pat_[A-Za-z0-9_]{20,})\b/ },
  { name: 'GitLab token', pattern: /\bglpat-[A-Za-z0-9_-]{16,}\b/ },
  { name: 'Slack token', pattern: /\bxox[baprs]-[A-Za-z0-9-]{10,}\b/ },
  { name: 'Anthropic key', pattern: /\bsk-ant-[A-Za-z0-9_-]{20,}\b/ },
  { name: 'OpenAI key', pattern: /\bsk-[A-Za-z0-9]{32,}\b/ },
  { name: 'Google API key', pattern: /\bAIza[0-9A-Za-z_-]{35}\b/ },
  { name: 'generic bearer', pattern: /\bBearer\s+[A-Za-z0-9._~+/-]{30,}={0,2}\b/ },
];

let secretFound = false;
for (const file of trackedFiles()) {
  let info;
  try {
    info = lstatSync(join(root, file));
  } catch {
    continue;
  }
  if (!info.isFile() || info.size > 2 * 1024 * 1024) continue;
  let buffer;
  try {
    buffer = readFileSync(join(root, file));
  } catch {
    continue;
  }
  if (buffer.subarray(0, 8000).includes(0)) continue;
  const text = buffer.toString('utf8');
  text.split(/\r?\n/).forEach((line, index) => {
    for (const rule of secretPatterns) {
      if (rule.pattern.test(line)) {
        secretFound = true;
        fail(`${file}:${index + 1}: 疑似 ${rule.name}（内容已隐藏）`);
        break;
      }
    }
  });
}
if (!secretFound) console.log('OK: 未发现常见密钥/令牌格式');

if (hasCommand('gitleaks')) {
  heading('gitleaks');
  // --no-git scans the current tree only; historical test fixtures should not
  // block a release, while committed secrets are still detected.
  const gitleaksArgs = ['detect', '--source', root, '--no-git', '--no-banner', '--redact'];
  const gitleaksConfig = join(root, '.gitleaks.toml');
  if (existsSync(gitleaksConfig)) gitleaksArgs.push('--config', gitleaksConfig);
  if (run('gitleaks', gitleaksArgs) !== 0) {
    fail('gitleaks 发现问题');
  } else {
    console.log('OK: gitleaks 未发现问题');
  }
} else {
  console.log('提示: 未安装 gitleaks，已使用内置扫描器');
}

// ---------------------------------------------------------------------------
// 3. Go static checks and vulnerabilities.

heading('go vet');
if (run('go', ['vet', './...']) !== 0) fail('go vet 失败');
else console.log('OK: go vet 通过');

if (!quick) {
  heading('govulncheck');
  if (!hasCommand('govulncheck')) {
    console.log('未找到 govulncheck，正在安装…');
    if (run('go', ['install', 'golang.org/x/vuln/cmd/govulncheck@latest']) !== 0) {
      fail('govulncheck 安装失败');
    } else {
      const goPath = capture('go', ['env', 'GOPATH']);
      if (goPath) {
        const pathKey = isWindows && Object.prototype.hasOwnProperty.call(process.env, 'Path') ? 'Path' : 'PATH';
        process.env[pathKey] = `${join(goPath, 'bin')}${isWindows ? ';' : ':'}${process.env[pathKey] ?? process.env.PATH ?? ''}`;
      }
    }
  }
  if (hasCommand('govulncheck')) {
    if (run('govulncheck', ['./...']) !== 0) fail('govulncheck 发现已知漏洞');
    else console.log('OK: govulncheck 通过');
  } else {
    fail('govulncheck 不可用');
  }
}

// ---------------------------------------------------------------------------
// 4. Frontend runtime dependency audit.

heading('npm audit（运行时依赖）');
if (run(command('npm'), ['audit', '--omit=dev', '--audit-level=high'], { cwd: join(root, 'frontend') }) !== 0) {
  fail('npm audit 发现 high/critical 漏洞或无法完成审计');
} else {
  console.log('OK: npm audit 通过');
}

// ---------------------------------------------------------------------------

if (failures > 0) {
  console.error(`\n安全检查失败：${failures} 项问题`);
  process.exit(1);
}
console.log('\n安全检查全部通过');
