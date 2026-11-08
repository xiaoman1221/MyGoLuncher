#!/usr/bin/env node
// Upload release artifacts to LicensingGo and publish a new OTA manifest.
// Files are uploaded through POST /api/v1/uploads/ota, so the manifest points
// at licensing-server storage instead of private GitHub release URLs.

import { createHash } from 'node:crypto';
import { existsSync, readFileSync, statSync } from 'node:fs';
import { basename, join } from 'node:path';

const args = process.argv.slice(2);
function argValue(name, fallback = '') {
  const index = args.indexOf(name);
  return index >= 0 && args[index + 1] ? args[index + 1] : fallback;
}

function envValue(...names) {
  for (const name of names) {
    const value = (process.env[name] ?? '').trim();
    if (value) return value;
  }
  return '';
}

function fail(message) {
  console.error(`ERROR: ${message}`);
  process.exit(1);
}

const version = (argValue('--version') || envValue('MGL_OTA_VERSION') || '').replace(/^v/, '').trim();
if (!/^\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$/.test(version)) {
  fail(`无效的 OTA 版本号: ${version || '(empty)'}`);
}

const server = envValue('MGL_PRODUCT_SERVER_URL', 'LICENSINGGO_BASE_URL').replace(/\/+$/, '');
if (!server) fail('缺少 MGL_PRODUCT_SERVER_URL');
let parsed;
try {
  parsed = new URL(server);
} catch {
  fail(`MGL_PRODUCT_SERVER_URL 不是有效 URL: ${server}`);
}
if (parsed.protocol !== 'https:' && !['localhost', '127.0.0.1'].includes(parsed.hostname)) {
  fail('授权系统地址必须使用 HTTPS');
}

const productID = envValue('MGL_PRODUCT_ID', 'LICENSINGGO_PRODUCT_ID');
const productSlug = envValue('MGL_PRODUCT_SLUG') || 'MyGo';
const productName = envValue('MGL_PRODUCT_NAME') || 'MyGoLuncher';
const artifactDir = envValue('MGL_OTA_ARTIFACT_DIR') || 'dist';
const allowPartial = /^(1|true|yes)$/i.test(envValue('MGL_OTA_ALLOW_PARTIAL'));
const updateTag = envValue('MGL_OTA_TAG').toLowerCase();
const forceTags = ['force', 'force_update', 'forced'];
const mandatoryTags = ['mandatory', 'recommended', 'important'];
const optionalTags = ['optional', 'normal', 'none'];
if (updateTag && ![...forceTags, ...mandatoryTags, ...optionalTags].includes(updateTag)) {
  fail(`无效的 MGL_OTA_TAG: ${updateTag}（可用 force / mandatory / optional）`);
}

let token = envValue('MGL_LICENSE_ADMIN_TOKEN', 'LICENSINGGO_CONSOLE_TOKEN');
const username = envValue('MGL_LICENSE_ADMIN_USERNAME', 'LICENSINGGO_ADMIN_USERNAME', 'MGL_ADMIN_USERNAME');
const password = envValue('MGL_LICENSE_ADMIN_PASSWORD', 'LICENSINGGO_ADMIN_PASSWORD', 'MGL_ADMIN_PASSWORD');
if (!token && (!username || !password)) {
  fail('缺少管理员凭据：请配置 MGL_LICENSE_ADMIN_USERNAME / MGL_LICENSE_ADMIN_PASSWORD 或 MGL_LICENSE_ADMIN_TOKEN');
}

function parseBool(value, fallback) {
  const raw = (value ?? '').trim().toLowerCase();
  if (raw === '') return fallback;
  return ['1', 'true', 'yes', 'on'].includes(raw);
}

async function request(path, options = {}) {
  const headers = { Accept: 'application/json', ...(options.headers ?? {}) };
  if (token) headers.Authorization = `Bearer ${token}`;
  const response = await fetch(`${server}${path}`, { ...options, headers });
  const text = await response.text();
  let body = null;
  if (text) {
    try {
      body = JSON.parse(text);
    } catch {
      body = text;
    }
  }
  if (!response.ok) {
    const detail = typeof body === 'string' ? body : JSON.stringify(body);
    throw new Error(`${options.method ?? 'GET'} ${path} -> HTTP ${response.status}: ${String(detail).slice(0, 600)}`);
  }
  return body;
}

async function login() {
  if (token) return;
  const result = await request('/api/v1/auth/login', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ username, password }),
  });
  token = (result?.token ?? '').trim();
  if (!token) fail('授权系统登录成功但未返回 token');
}

function channelPlatforms(channel) {
  const out = {};
  if (!channel || typeof channel !== 'object') return out;
  // Legacy single-artifact channel: {"version","url","sha256","size"}.
  const hasNested = Object.keys(channel).some((key) => !['version', 'url', 'sha256', 'size'].includes(key));
  if (!hasNested && channel.url && channel.sha256 && Number(channel.size) > 0) {
    out.any = {
      any: {
        version: String(channel.version ?? version),
        url: String(channel.url),
        sha256: String(channel.sha256).toLowerCase(),
        size: Number(channel.size),
      },
    };
  }
  for (const [os, arches] of Object.entries(channel)) {
    if (['version', 'url', 'sha256', 'size'].includes(os)) continue;
    if (!arches || typeof arches !== 'object') continue;
    const clean = {};
    for (const [arch, artifact] of Object.entries(arches)) {
      if (artifact && artifact.url && artifact.sha256 && Number(artifact.size) > 0) {
        clean[arch] = {
          version: String(artifact.version ?? version),
          url: String(artifact.url),
          sha256: String(artifact.sha256).toLowerCase(),
          size: Number(artifact.size),
        };
      }
    }
    if (Object.keys(clean).length > 0) out[os] = clean;
  }
  return out;
}

function sha256(path) {
  return createHash('sha256').update(readFileSync(path)).digest('hex');
}

const artifacts = [
  {
    kind: 'app', os: 'windows', arch: 'x64', required: true,
    file: join(artifactDir, `MyGoLuncher-${version}-windows-amd64.exe`),
  },
  {
    kind: 'app', os: 'macos', arch: 'universal', required: true,
    file: join(artifactDir, `MyGoLuncher-${version}-macos-universal.zip`),
  },
  {
    kind: 'app', os: 'linux', arch: 'x64', required: true,
    file: join(artifactDir, `MyGoLuncher-${version}-linux-amd64.tar.gz`),
  },
  {
    kind: 'web', os: 'any', arch: 'any', required: false,
    file: join(artifactDir, `MyGoLuncher-${version}-web.zip`),
  },
];

const present = artifacts.filter((artifact) => existsSync(artifact.file));
for (const artifact of artifacts) {
  if (artifact.required && !existsSync(artifact.file)) {
    if (allowPartial) {
      console.warn(`WARNING: 缺少 ${artifact.file}，按 MGL_OTA_ALLOW_PARTIAL 继续`);
    } else {
      fail(`缺少构建产物: ${artifact.file}`);
    }
  }
}
if (present.length === 0) fail('没有任何可上传的构建产物');

await login();

let product = null;
if (productID) {
  product = await request(`/api/v1/products/${encodeURIComponent(productID)}`);
} else {
  const list = await request(`/api/v1/products?q=${encodeURIComponent(productSlug)}`);
  const items = Array.isArray(list) ? list : Array.isArray(list?.products) ? list.products : [];
  product = items.find((item) => String(item.slug ?? '').toLowerCase() === productSlug.toLowerCase())
    || items.find((item) => String(item.name ?? '') === productName)
    || null;
  if (!product) fail(`授权系统中找不到产品 slug=${productSlug} name=${productName}，请设置 MGL_PRODUCT_ID`);
}
const resolvedProductID = String(product.id ?? '').trim();
if (!resolvedProductID) fail('无法确定授权系统产品 ID');

const existing = product.ota && typeof product.ota === 'object' ? product.ota : {};
const appPlatforms = channelPlatforms(existing.app);
const webPlatforms = channelPlatforms(existing.web);

console.log(`产品: ${product.name ?? productName} (${resolvedProductID})`);
console.log(`版本: ${version}`);

for (const artifact of present) {
  const localHash = sha256(artifact.file);
  const localSize = statSync(artifact.file).size;
  const form = new FormData();
  form.append('product_id', resolvedProductID);
  form.append('kind', artifact.kind);
  form.append('file', new Blob([readFileSync(artifact.file)], { type: 'application/octet-stream' }), basename(artifact.file));

  const uploaded = await request('/api/v1/uploads/ota', { method: 'POST', body: form });
  if (!uploaded?.url || !uploaded?.sha256 || !uploaded?.size) {
    fail(`上传 ${basename(artifact.file)} 后服务端返回不完整`);
  }
  if (String(uploaded.sha256).toLowerCase() !== localHash || Number(uploaded.size) !== localSize) {
    fail(`上传 ${basename(artifact.file)} 后校验不一致`);
  }
  let downloadURL;
  try {
    downloadURL = new URL(String(uploaded.url));
  } catch {
    fail(`上传 ${basename(artifact.file)} 后返回的下载地址无效`);
  }
  if (downloadURL.protocol !== 'https:' && !['localhost', '127.0.0.1'].includes(downloadURL.hostname)) {
    fail(`上传 ${basename(artifact.file)} 后返回的下载地址不是 HTTPS: ${uploaded.url}`);
  }

  const entry = {
    version,
    url: String(uploaded.url),
    sha256: localHash,
    size: localSize,
  };
  if (artifact.kind === 'web') {
    webPlatforms[artifact.os] = webPlatforms[artifact.os] ?? {};
    webPlatforms[artifact.os][artifact.arch] = entry;
  } else {
    appPlatforms[artifact.os] = appPlatforms[artifact.os] ?? {};
    appPlatforms[artifact.os][artifact.arch] = entry;
  }
  console.log(`已上传 ${artifact.kind}/${artifact.os}/${artifact.arch}: ${uploaded.url}`);
}

const notes = envValue('MGL_OTA_NOTES') || String(existing.notes ?? '') || `MyGoLuncher v${version}`;
// TAG policy:
//   force     -> 强制更新（客户端全局弹窗，不可跳过）
//   mandatory -> 非强制的重要更新（普通提示）
//   optional  -> 普通可选更新
let taggedMandatory = Boolean(existing.mandatory);
let taggedForce = Boolean(existing.force);
if (forceTags.includes(updateTag)) {
  taggedMandatory = true;
  taggedForce = true;
} else if (mandatoryTags.includes(updateTag)) {
  taggedMandatory = true;
  taggedForce = false;
} else if (optionalTags.includes(updateTag)) {
  taggedMandatory = false;
  taggedForce = false;
}
const manifest = {
  version,
  minVersion: envValue('MGL_OTA_MIN_VERSION') || String(existing.minVersion ?? ''),
  notes,
  // Explicit MGL_OTA_MANDATORY / MGL_OTA_FORCE override the tag.
  mandatory: parseBool(envValue('MGL_OTA_MANDATORY'), taggedMandatory),
  force: parseBool(envValue('MGL_OTA_FORCE'), taggedForce),
  app: appPlatforms,
};
if (Object.keys(webPlatforms).length > 0) manifest.web = webPlatforms;

await request(`/api/v1/products/${encodeURIComponent(resolvedProductID)}/ota`, {
  method: 'PUT',
  headers: { 'Content-Type': 'application/json' },
  body: JSON.stringify(manifest),
});
console.log(`OTA 清单已更新: ${server}/api/v1/update (version ${version})`);
