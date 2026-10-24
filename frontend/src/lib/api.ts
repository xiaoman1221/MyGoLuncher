import { mockCall } from './mock';
import type {
  AccountLoginEvent,
  AccountState,
  AccountView,
  AuthlibServer,
  Bootstrap,
  MirrorConfig,
  MirrorSpeedResult,
  CreateInstanceRequest,
  DeviceCodeView,
  ExternalImportResponse,
  ExternalScanResult,
  Hitokoto,
  InstallProjectResult,
  InstanceView,
  JavaPackage,
  JavaReleaseInfo,
  JavaRuntime,
  ModpackImportResponse,
  ProjectVersion,
  ResourceSearchRequest,
  ResourceSearchResult,
  ResourceVersion,
  SearchRequest,
  SearchResult,
  Settings,
  UpdateInstanceRequest,
  VersionList,
} from './types';

type WailsWindow = Window & {
  go?: {
    main?: {
      App?: Record<string, (...args: unknown[]) => Promise<unknown>>;
    };
  };
  runtime?: {
    EventsOn?: (name: string, callback: (payload: unknown) => void) => () => void;
  };
};

const host = window as WailsWindow;

export const isDesktop = Boolean(host.go?.main?.App?.Bootstrap);
export const isDemo = !isDesktop;

async function call<T>(method: string, ...args: unknown[]): Promise<T> {
  if (isDesktop) {
    const fn = host.go?.main?.App?.[method];
    if (!fn) throw new Error(`客户端绑定缺失: ${method}`);
    return fn(...args) as Promise<T>;
  }
  return mockCall<T>(method, args);
}

export const api = {
  bootstrap: () => call<Bootstrap>('Bootstrap'),
  getInstances: () => call<InstanceView[]>('GetInstances'),
  saveSettings: (settings: Settings) => call<Settings>('SaveSettings', settings),
  getHitokoto: () => call<Hitokoto>('GetHitokoto'),
  testDownloadMirrors: (customs: MirrorConfig[]) =>
    call<MirrorSpeedResult[]>('TestDownloadMirrors', customs),

  getAccounts: () => call<AccountState>('GetAccounts'),
  selectAccount: (id: string) => call<AccountState>('SelectAccount', id),
  addOfflineAccount: (name: string) => call<AccountState>('AddOfflineAccount', name),
  renameOfflineAccount: (id: string, name: string) =>
    call<AccountState>('RenameOfflineAccount', id, name),
  removeAccount: (id: string) => call<AccountState>('RemoveAccount', id),
  loginAuthlib: (serverUrl: string, username: string, password: string) =>
    call<AccountView>('LoginAuthlib', serverUrl, username, password),
  authlibServers: () => call<AuthlibServer[]>('AuthlibServers'),
  beginMicrosoftLogin: () => call<DeviceCodeView>('BeginMicrosoftLogin'),
  cancelMicrosoftLogin: () => call<void>('CancelMicrosoftLogin'),

  listVersions: (includeSnapshots: boolean, includeOld: boolean) =>
    call<VersionList>('ListVersions', includeSnapshots, includeOld),
  installVersion: (id: string) => call<void>('InstallVersion', id),

  createInstance: (request: CreateInstanceRequest) =>
    call<InstanceView>('CreateInstance', request),
  updateInstance: (request: UpdateInstanceRequest) =>
    call<InstanceView>('UpdateInstance', request),
  deleteInstance: (id: string, deleteFiles: boolean) =>
    call<void>('DeleteInstance', id, deleteFiles),

  launchInstance: (id: string) => call<void>('LaunchInstance', id),
  stopInstance: (id: string) => call<void>('StopInstance', id),
  getInstanceLog: (id: string) => call<string[]>('GetInstanceLog', id),
  openInstanceFolder: (id: string) => call<void>('OpenInstanceFolder', id),
  openDataFolder: () => call<void>('OpenDataFolder'),
  openConfigFolder: () => call<void>('OpenConfigFolder'),
  openURL: (url: string) => call<void>('OpenURL', url),

  searchResources: (request: ResourceSearchRequest) =>
    call<ResourceSearchResult>('SearchResources', request),
  listResourceVersions: (instanceId: string, source: string, projectId: string, type: string) =>
    call<ResourceVersion[]>('ListResourceVersions', instanceId, source, projectId, type),
  installResource: (instanceId: string, source: string, projectId: string, versionId: string) =>
    call<InstallProjectResult>('InstallResource', instanceId, source, projectId, versionId),
  installResourceModpack: (source: string, projectId: string, versionId: string) =>
    call<ModpackImportResponse>('InstallResourceModpack', source, projectId, versionId),
  searchProjects: (request: SearchRequest) => call<SearchResult>('SearchProjects', request),
  listProjectVersions: (instanceId: string, projectId: string) =>
    call<ProjectVersion[]>('ListProjectVersions', instanceId, projectId),
  installProject: (instanceId: string, projectId: string, versionId: string) =>
    call<InstallProjectResult>('InstallProject', instanceId, projectId, versionId),
  importModpackDialog: () => call<ModpackImportResponse>('ImportModpackDialog'),
  installModpackProject: (projectId: string, versionId: string) =>
    call<ModpackImportResponse>('InstallModpackProject', projectId, versionId),
  exportModpackDialog: (instanceId: string) =>
    call<string>('ExportModpackDialog', instanceId),
  exportInstanceBackupDialog: (instanceId: string) =>
    call<string>('ExportInstanceBackupDialog', instanceId),
  pickExternalLauncher: () => call<ExternalScanResult>('PickExternalLauncher'),
  pickVersionDirectory: () => call<ExternalScanResult>('PickVersionDirectory'),
  scanVersionDirectory: (path: string) => call<ExternalScanResult>('ScanVersionDirectory', path),
  detectLocalLaunchers: () => call<ExternalScanResult[]>('DetectLocalLaunchers'),
  importExternalInstance: (root: string, candidateId: string, name: string) =>
    call<ExternalImportResponse>('ImportExternalInstance', root, candidateId, name),

  getJavaRuntimes: () => call<JavaRuntime[]>('GetJavaRuntimes'),
  getJavaReleases: () => call<JavaReleaseInfo>('GetJavaReleases'),
  getJavaPackage: (feature: number) => call<JavaPackage>('GetJavaPackage', feature),
  installJava: (feature: number) => call<JavaRuntime>('InstallJava', feature),
};
