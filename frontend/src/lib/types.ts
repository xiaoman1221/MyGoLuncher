export interface MirrorConfig {
  id: string;
  name: string;
  url: string;
  kind: 'bmclapi' | 'proxy' | string;
}

export interface MirrorSpeedResult {
  id: string;
  name: string;
  url: string;
  ok: boolean;
  error?: string;
  latencyMs: number;
  bytes: number;
  bytesPerSec: number;
}

export interface Settings {
  playerName: string;
  defaultMemoryMB: number;
  defaultJavaPath: string;
  curseForgeAPIKey: string;
  microsoftClientID: string;
  extraJvmArgs: string;
  customMirrors: MirrorConfig[];
  downloadWorkers: number;
  showGameConsole: boolean;
  showLauncherOnLaunch: boolean;
  hitokotoApiUrl: string;
  languageAuto: boolean;
  language: string;
  versionIsolation: boolean;
  aiSearchEnabled: boolean;
  aiSearchProvider: string;
  aiSearchBaseUrl: string;
  aiSearchApiKey: string;
  aiSearchModel: string;
  schemaVersion?: number;
}

export interface Hitokoto {
  text: string;
  type: string;
  from: string;
  fromWho: string;
}

export interface Instance {
  id: string;
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion: string;
  launchVersionId: string;
  memoryMB: number;
  javaPath: string;
  createdAt: string;
  lastPlayed: string;
  playTimeMinutes: number;
  source: string;
  notes: string;
}

export interface InstanceView extends Instance {
  gameDir: string;
  requiredJava: number;
  versionInstalled: boolean;
  running: boolean;
  modCount: number;
  shaderCount: number;
  resourcePackCount: number;
  hasJavaOverride: boolean;
  autoMemory: boolean;
  effectiveMemoryMB: number;
}

export interface PathsView {
  base: string;
  root: string;
  data: string;
  instances: string;
  versions: string;
  libraries: string;
  assets: string;
  runtimes: string;
  cache: string;
}

export interface Bootstrap {
  settings: Settings;
  instances: InstanceView[];
  paths: PathsView;
  version: string;
  initError: string;
  running: string[];
  accounts: AccountView[];
  selectedAccountId: string;
  totalMemoryMB: number;
  autoMemoryMB: number;
}

export type AccountKind = 'offline' | 'microsoft' | 'authlib';

export interface AccountView {
  id: string;
  kind: AccountKind;
  name: string;
  uuid: string;
  serverUrl: string;
  expiresAt: string;
  label: string;
}

export interface AccountState {
  accounts: AccountView[];
  selectedId: string;
}

export interface DeviceCodeView {
  userCode: string;
  verificationUri: string;
  expiresIn: number;
  interval: number;
  message: string;
}

export interface YSSDeviceView {
  userCode: string;
  verificationUri: string;
  verificationUriComplete: string;
  expiresIn: number;
  interval: number;
  message: string;
}

export interface AccountLoginEvent {
  success: boolean;
  message: string;
  account?: AccountView;
}

export interface VersionView {
  id: string;
  type: 'release' | 'snapshot' | 'old_beta' | 'old_alpha' | string;
  releaseTime: string;
  javaMajor: number;
  installed: boolean;
}

export interface VersionList {
  versions: VersionView[];
  latestRelease: string;
  latestSnapshot: string;
}

export interface TaskProgress {
  taskId: string;
  title: string;
  stage: string;
  message: string;
  current: number;
  total: number;
  percent: number;
  done: boolean;
  failed: boolean;
}

export interface LaunchState {
  instanceId: string;
  running: boolean;
  exitCode: number;
  durationSeconds: number;
  stoppedByUser: boolean;
}

export interface SearchRequest {
  query: string;
  projectType: string;
  gameVersion: string;
  loader: string;
  offset: number;
  limit: number;
  sort: string;
}

export interface Project {
  project_id: string;
  project_type: string;
  slug: string;
  title: string;
  description: string;
  author: string;
  categories: string[];
  downloads: number;
  follows: number;
  icon_url: string;
  versions: string[];
  latest_version: string;
  color: number;
  date_modified?: string;
}

export interface SearchResult {
  hits: Project[];
  total_hits: number;
  offset: number;
  limit: number;
}

export interface ProjectFile {
  url: string;
  filename: string;
  primary: boolean;
  size: number;
  hashes: Record<string, string>;
}

export interface ProjectVersion {
  id: string;
  project_id: string;
  name: string;
  version_number: string;
  game_versions: string[];
  loaders: string[];
  date_published: string;
  version_type: string;
  downloads: number;
  files: ProjectFile[];
  dependencies: Array<{
    version_id: string | null;
    project_id: string | null;
    file_name: string | null;
    dependency_type: string;
  }>;
}

export interface JavaRuntime {
  path: string;
  home: string;
  version: string;
  major: number;
  vendor: string;
  architecture: string;
  managed: boolean;
  source: string;
}

export interface JavaReleaseInfo {
  available: number[];
  lts: number[];
  latestFeature: number;
}

export interface JavaPackage {
  feature: number;
  name: string;
  url: string;
  sha256: string;
  size: number;
  version: string;
  releaseName: string;
  os: string;
  arch: string;
  installed: boolean;
}

export interface ServerPackExportResult {
  path: string;
  warnings: string[];
}

export interface ModpackImportResponse {
  instance: InstanceView;
  format: string;
  formatLabel: string;
  missingFiles: number;
  warnings: string[];
}

export interface InstallProjectResult {
  fileName: string;
  path: string;
  versionId: string;
  versionName: string;
  gameVersion: string;
  loader: string;
  dependencies: string[];
}

export interface ExternalCandidate {
  id: string;
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion: string;
  source: string;
  modCount: number;
  hasSaves: boolean;
  warnings: string[];
}

export interface ExternalScanResult {
  root: string;
  source: string;
  candidates: ExternalCandidate[];
}

export interface ExternalImportResponse {
  instance: InstanceView;
  source: string;
  filesCopied: number;
  modsCopied: number;
  resourcePacksCopied: number;
  shaderPacksCopied: number;
  savesCopied: number;
  warnings: string[];
}

export interface ResourceProject {
  id: string;
  projectId: string;
  source: ResourceSource;
  type: ResourceType;
  slug: string;
  title: string;
  description: string;
  author: string;
  iconUrl: string;
  websiteUrl: string;
  downloads: number;
  categories: string[];
  gameVersions: string[];
  latestVersion: string;
  color: number;
  dateModified: string;
}

export interface ResourceVersion {
  id: string;
  projectId: string;
  source: ResourceSource;
  name: string;
  versionNumber: string;
  releaseType: string;
  datePublished: string;
  gameVersions: string[];
  loaders: string[];
  downloads: number;
  fileName: string;
  fileSize: number;
  downloadable: boolean;
}

export interface ResourceSearchRequest {
  instanceId: string;
  query: string;
  type: ResourceType;
  source: 'all' | ResourceSource;
  sort: string;
  offset: number;
  limit: number;
  useAI: boolean;
}

export interface ResourceSearchResult {
  items: ResourceProject[];
  total: number;
  offset: number;
  limit: number;
  warnings: string[];
  curseForgeReady: boolean;
  queryUsed: string;
  aiUsed: boolean;
  aiError: string;
  cjkQuery: boolean;
}

export interface CreateInstanceRequest {
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion: string;
  memoryMB: number;
}

export interface UpdateInstanceRequest {
  id: string;
  name: string;
  memoryMB: number;
  javaPath: string;
}

export interface AuthlibServer {
  id: string;
  name: string;
  url: string;
}

export type PageKey = 'home' | 'versions' | 'instanceSettings' | 'resources' | 'modpacks' | 'settings';
export type Theme = 'dark' | 'light';
export type ResourceType = 'mod' | 'shader' | 'resourcepack' | 'datapack' | 'modpack';
export type ResourceSource = 'modrinth' | 'curseforge';
