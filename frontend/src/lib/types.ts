export interface MirrorConfig {
  id: string;
  name: string;
  url: string;
  kind: 'bmclapi' | 'proxy' | 'mcim' | string;
}

export interface ResourceMirror {
  id: string;
  name: string;
  baseUrl: string;
  kind: 'official' | 'mcim' | 'proxy' | string;
  official: boolean;
  custom: boolean;
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
  resourceMirror: string;
  customResourceMirrors: MirrorConfig[];
  serverBackupDisabled: boolean;
  downloadWorkers: number;
  showGameConsole: boolean;
  showLauncherOnLaunch: boolean;
  closeBehavior: 'ask' | 'minimize' | 'exit';
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

export type InstanceResourceKind = 'mods' | 'resourcepacks' | 'shaderpacks';

export interface InstanceResource {
  kind: InstanceResourceKind;
  fileName: string;
  name: string;
  enabled: boolean;
  directory: boolean;
  size: number;
  modifiedAt?: string;
}

export interface InstanceResources {
  mods: InstanceResource[];
  resourcePacks: InstanceResource[];
  shaders: InstanceResource[];
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

export interface LicenseStatus {
  state: 'inactive' | 'active' | 'grace' | 'expired' | 'invalid' | 'error' | 'machine_changed' | 'trial' | 'trial_expired';
  configured: boolean;
  activated: boolean;
  code?: string;
  machineCode: string;
  email?: string;
  customer?: string;
  canSelfUnbind?: boolean;
  expiresAt?: string;
  features: string[];
  lastChecked?: string;
  graceUntil?: string;
  message?: string;
  graceDays: number;
  trial?: boolean;
  trialRemaining?: number;
  trialDaysLeft?: number;
  trialDays?: number;
  trialEndsAt?: string;
}

export interface UpdateInfo {
  configured: boolean;
  currentVersion: string;
  version?: string;
  minVersion?: string;
  notes?: string;
  mandatory: boolean;
  force?: boolean;
  available: boolean;
  hasWeb: boolean;
  hasApp: boolean;
  webVersion?: string;
  appVersion?: string;
  publishedAt?: string;
}

export interface Bootstrap {
  license: LicenseStatus;
  elevated: boolean;
  elevationConfirmed: boolean;
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

export interface LoaderVersionInfo {
  version: string;
  stable: boolean;
  recommended: boolean;
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
  canceled: boolean;
}

export interface TaskLogEntry {
  seq: number;
  time: string;
  stage: string;
  message: string;
  current: number;
  total: number;
  done: boolean;
  failed: boolean;
  canceled: boolean;
}

export interface TaskLog {
  taskId: string;
  title: string;
  done: boolean;
  failed: boolean;
  canceled: boolean;
  entries: TaskLogEntry[];
}

export interface TaskLogEvent {
  taskId: string;
  entry: TaskLogEntry;
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

export interface ModUpdateFile {
  fileName: string;
  relativePath: string;
  name: string;
  currentVersion: string;
  source?: string;
  projectId?: string;
  versionId?: string;
  latestVersion?: string;
  latestName?: string;
  fileSize: number;
  updateAvailable: boolean;
  identified: boolean;
  error?: string;
}

export interface ModUpdateReport {
  instanceId: string;
  files: ModUpdateFile[];
  identified: number;
  updates: number;
  unidentified: number;
  warnings: string[];
}

export interface ModUpdateApplyRequest {
  instanceId: string;
  relativePath: string;
  source: string;
  projectId: string;
  versionId: string;
  force: boolean;
}

export interface InstallProjectResult {
  fileName: string;
  path: string;
  versionId: string;
  versionName: string;
  gameVersion: string;
  loader: string;
  dependencies: string[];
  rollbackPath?: string;
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
  contentDir?: string;
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

export interface CustomImportRequest {
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion: string;
  sourceDir: string;
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
  queryAlternatives?: string[];
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

export type ServerSyncDir =
  | 'mods'
  | 'config'
  | 'defaultconfigs'
  | 'scripts'
  | 'kubejs'
  | 'datapacks'
  | 'resourcepacks'
  | 'shaderpacks';

export interface ServerBackup {
  id: string;
  createdAt: string;
  reason?: string;
  dirs: ServerSyncDir[];
  missingDirs?: ServerSyncDir[];
  emptyDirs?: ServerSyncDir[];
}

export interface ServerProfile {
  id: string;
  name: string;
  panelUrl: string;
  daemonId: string;
  instanceUuid: string;
  localInstanceId: string;
  remoteRoot: string;
  syncDirs: ServerSyncDir[];
  deleteExtras: boolean;
  createdAt?: string;
  lastSync?: string;
  lastRollback?: string;
  backups?: ServerBackup[];
  apiKeySet: boolean;
}

export interface ServerProfileRequest {
  id?: string;
  name: string;
  panelUrl: string;
  daemonId: string;
  instanceUuid: string;
  localInstanceId: string;
  remoteRoot: string;
  syncDirs: ServerSyncDir[];
  deleteExtras: boolean;
  apiKey?: string;
  clearApiKey?: boolean;
}

export interface McsmConnectRequest {
  profileId?: string;
  panelUrl?: string;
  apiKey?: string;
}

export interface RemoteMcsmInstance {
  daemonId: string;
  instanceUuid: string;
  nickname: string;
  status: number;
}

export interface ServerStatus {
  profileId: string;
  online: boolean;
  status: number;
  statusText: string;
  nickname: string;
  error?: string;
}

export interface ServerSyncResult {
  uploaded: number;
  skipped: number;
  deleted: number;
  filtered: number;
  bytes: number;
}

export interface ServerSyncFile {
  path: string;
  dir: ServerSyncDir;
  size: number;
  clientOnly: boolean;
  reason?: string;
}

export interface LanPlayer {
  name: string;
}

export interface LanWorld {
  id: string;
  host: string;
  port: number;
  address: string;
  motd: string;
  version: string;
  online: number;
  max: number;
  players: LanPlayer[];
  source: 'lan' | 'scan' | string;
  hostName?: string;
  lastSeen: string;
}

export type LanShareDir = 'mods' | 'shaderpacks' | 'resourcepacks' | 'config';

export interface LanPeer {
  id: string;
  name: string;
  instance: string;
  mcVersion: string;
  address: string;
  instanceId?: string;
  loader?: string;
  loaderVersion?: string;
  syncPort?: number;
  syncDirs?: string[];
  canSync: boolean;
  canChat: boolean;
  lastSeen: string;
}

export type LanToolKind = 'chat' | 'voice' | 'file';

export interface LanToolMessage {
  id: string;
  kind: LanToolKind;
  fromId: string;
  fromName: string;
  text?: string;
  chatId?: string;
  chatName?: string;
  members?: string[];
  fileName?: string;
  fileId?: string;
  size?: number;
  mimeType?: string;
  durationMs?: number;
  progress?: number;
  downloaded?: boolean;
  path?: string;
  createdAt: string;
  incoming: boolean;
}

export interface LanSignal {
  seq: number;
  fromId: string;
  fromName: string;
  session: string;
  kind: "offer" | "answer" | "ice";
  payload: string;
  createdAt: string;
}

export interface LanSignalBatch {
  signals: LanSignal[];
  lastSeq: number;
}

export interface LanChatGroup {
  id: string;
  name: string;
  peerIds: string[];
}

export interface LanToolState {
  selfId: string;
  selfName: string;
  port: number;
  inbox: string;
  messages: LanToolMessage[];
}

export interface LanToolSendResult {
  message: LanToolMessage;
  count: number;
  warning?: string;
}

export interface LanFileInfo {
  path: string;
  name: string;
  size: number;
}

export interface LanSnapshot {
  running: boolean;
  broadcast: boolean;
  sharing: boolean;
  sharedInstanceId?: string;
  shareDirs?: string[];
  worlds: LanWorld[];
  peers: LanPeer[];
  warnings?: string[];
}

export interface LanInstanceInfo {
  id: string;
  name: string;
  mcVersion: string;
  loader: string;
  loaderVersion: string;
  launchVersionId: string;
}

export interface LanSyncFileChange {
  path: string;
  state: 'new' | 'changed';
  reason?: string;
  localSize: number;
  remoteSize: number;
}

export interface LanSyncPlan {
  totalFiles: number;
  newFiles: number;
  changedFiles: number;
  sameFiles: number;
  extraFiles: number;
  newBytes: number;
  changedBytes: number;
  changes: LanSyncFileChange[];
  extras: string[];
  digest: string;
}

export interface LanSyncPreview {
  peer: LanInstanceInfo;
  target: LanInstanceInfo;
  dirs: string[];
  plan: LanSyncPlan;
  digest: string;
  versionMismatch: boolean;
  loaderMismatch: boolean;
  loaderVersionMismatch: boolean;
  runtimeMissing: boolean;
  running: boolean;
  blocking: boolean;
  blockingReason?: string;
  warnings: string[];
}

export interface LanSyncResult {
  downloaded: number;
  skipped: number;
  bytes: number;
}

export interface LanVersionPreview {
  peer: LanInstanceInfo;
  files: number;
  bytes: number;
  dirs: string[];
  digest: string;
  warnings: string[];
}

export interface LanVersionInstallResult {
  sync: LanSyncResult;
  instance: InstanceView;
}

export type PageKey =
  | 'home'
  | 'versions'
  | 'instanceSettings'
  | 'resources'
  | 'modpacks'
  | 'servers'
  | 'lan'
  | 'settings'
  | 'about';
export type Theme = 'dark' | 'light';
export type ResourceType = 'mod' | 'shader' | 'resourcepack' | 'datapack' | 'modpack';
export type ResourceSource = 'modrinth' | 'curseforge';
