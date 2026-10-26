import type {
  AccountLoginEvent,
  AccountState,
  AccountView,
  AuthlibServer,
  Bootstrap,
  MirrorConfig,
  MirrorSpeedResult,
  ResourceMirror,
  CreateInstanceRequest,
  CustomImportRequest,
  DeviceCodeView,
  ExternalImportResponse,
  ExternalScanResult,
  Hitokoto,
  InstallProjectResult,
  InstanceView,
  JavaPackage,
  JavaReleaseInfo,
  JavaRuntime,
  LanFileInfo,
  LanSnapshot,
  LanSyncPreview,
  LanSyncResult,
  LanToolSendResult,
  LanToolState,
  LanVersionInstallResult,
  LanVersionPreview,
  LicenseStatus,
  LoaderVersionInfo,
  McsmConnectRequest,
  ModpackImportResponse,
  ModUpdateApplyRequest,
  ModUpdateReport,
  ProjectVersion,
  ResourceSearchRequest,
  ResourceSearchResult,
  ResourceVersion,
  RemoteMcsmInstance,
  SearchRequest,
  SearchResult,
  ServerPackExportResult,
  ServerProfile,
  ServerProfileRequest,
  ServerStatus,
  ServerSyncFile,
  ServerSyncResult,
  ServerBackup,
  Settings,
  TaskLog,
  UpdateInfo,
  UpdateInstanceRequest,
  VersionList,
  YSSDeviceView,
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
  // The demo backend is loaded on demand so its large fixture set is not part
  // of the production desktop bundle.
  const { mockCall } = await import('./mock');
  return mockCall<T>(method, args);
}

export const api = {
  bootstrap: () => call<Bootstrap>('Bootstrap'),
  getInstances: () => call<InstanceView[]>('GetInstances'),
  saveSettings: (settings: Settings) => call<Settings>('SaveSettings', settings),
  getHitokoto: () => call<Hitokoto>('GetHitokoto'),
  beginOrcaRouterLogin: () => call<void>('BeginOrcaRouterLogin'),
  cancelOrcaRouterLogin: () => call<void>('CancelOrcaRouterLogin'),
  testDownloadMirrors: (customs: MirrorConfig[]) =>
    call<MirrorSpeedResult[]>('TestDownloadMirrors', customs),
  listResourceMirrors: () => call<ResourceMirror[]>('ListResourceMirrors'),

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
  beginYSSLogin: (serverUrl: string) => call<YSSDeviceView>('BeginYSSLogin', serverUrl),
  cancelYSSLogin: () => call<void>('CancelYSSLogin'),

  listVersions: (includeSnapshots: boolean, includeOld: boolean) =>
    call<VersionList>('ListVersions', includeSnapshots, includeOld),
  installVersion: (id: string) => call<void>('InstallVersion', id),
  repairVersion: (instanceId: string) => call<void>('RepairVersion', instanceId),
  getLoaderVersions: (loaderType: string, mcVersion: string) =>
    call<LoaderVersionInfo[]>('GetLoaderVersions', loaderType, mcVersion),

  createInstance: (request: CreateInstanceRequest) =>
    call<InstanceView>('CreateInstance', request),
  updateInstance: (request: UpdateInstanceRequest) =>
    call<InstanceView>('UpdateInstance', request),
  deleteInstance: (id: string, deleteFiles: boolean) =>
    call<void>('DeleteInstance', id, deleteFiles),

  launchInstance: (id: string, language: string) => call<void>('LaunchInstance', id, language),
  launchInstanceToServer: (id: string, language: string, address: string) =>
    call<void>('LaunchInstanceToServer', id, language, address),
  stopInstance: (id: string) => call<void>('StopInstance', id),
  getTaskLog: (taskId: string) => call<TaskLog>('GetTaskLog', taskId),
  cancelTask: (taskId: string) => call<void>('CancelTask', taskId),
  getLicenseStatus: () => call<LicenseStatus>('GetLicenseStatus'),
  activateLicense: (code: string) => call<LicenseStatus>('ActivateLicense', code),
  startTrial: () => call<LicenseStatus>('StartTrial'),
  validateLicense: () => call<LicenseStatus>('ValidateLicense'),
  unbindLicense: () => call<LicenseStatus>('UnbindLicense'),
  confirmElevation: (allow: boolean) => call<void>('ConfirmElevation', allow),
  checkForUpdates: () => call<UpdateInfo>('CheckForUpdates'),
  applyWebUpdate: () => call<string>('ApplyWebUpdate'),
  applyAppUpdate: () => call<string>('ApplyAppUpdate'),
  rollbackWebUpdate: () => call<void>('RollbackWebUpdate'),
  checkModUpdates: (instanceId: string) => call<ModUpdateReport>('CheckModUpdates', instanceId),
  applyModUpdate: (request: ModUpdateApplyRequest) =>
    call<InstallProjectResult>('ApplyModUpdate', request),
  getInstanceLog: (id: string) => call<string[]>('GetInstanceLog', id),
  openInstanceFolder: (id: string) => call<void>('OpenInstanceFolder', id),
  openInstanceSubfolder: (id: string, folder: string) => call<void>('OpenInstanceSubfolder', id, folder),
  openDataFolder: () => call<void>('OpenDataFolder'),
  openConfigFolder: () => call<void>('OpenConfigFolder'),
  openURL: (url: string) => call<void>('OpenURL', url),
  requestClose: () => call<void>('RequestClose'),
  quitApp: () => call<void>('QuitApp'),
  minimizeWindow: () => call<void>('MinimizeWindow'),

  searchResources: (request: ResourceSearchRequest) =>
    call<ResourceSearchResult>('SearchResources', request),
  listResourceVersions: (instanceId: string, source: string, projectId: string, type: string) =>
    call<ResourceVersion[]>('ListResourceVersions', instanceId, source, projectId, type),
  installResource: (instanceId: string, source: string, projectId: string, versionId: string, mirrorId = '', force = false) =>
    call<InstallProjectResult>('InstallResource', instanceId, source, projectId, versionId, mirrorId, force),
  installResourceModpack: (source: string, projectId: string, versionId: string, mirrorId = '') =>
    call<ModpackImportResponse>('InstallResourceModpack', source, projectId, versionId, mirrorId),
  searchProjects: (request: SearchRequest) => call<SearchResult>('SearchProjects', request),
  listProjectVersions: (instanceId: string, projectId: string) =>
    call<ProjectVersion[]>('ListProjectVersions', instanceId, projectId),
  installProject: (instanceId: string, projectId: string, versionId: string) =>
    call<InstallProjectResult>('InstallProject', instanceId, projectId, versionId),
  importModpackDialog: (mirrorId = '') => call<ModpackImportResponse>('ImportModpackDialog', mirrorId),
  installModpackProject: (projectId: string, versionId: string, mirrorId = '') =>
    call<ModpackImportResponse>('InstallModpackProject', projectId, versionId, mirrorId),
  exportModpackDialog: (instanceId: string) =>
    call<string>('ExportModpackDialog', instanceId),
  exportInstanceBackupDialog: (instanceId: string) =>
    call<string>('ExportInstanceBackupDialog', instanceId),
  exportServerPackDialog: (instanceId: string) =>
    call<ServerPackExportResult>('ExportServerPackDialog', instanceId),
  pickExternalLauncher: () => call<ExternalScanResult>('PickExternalLauncher'),
  pickVersionDirectory: () => call<ExternalScanResult>('PickVersionDirectory'),
  scanVersionDirectory: (path: string) => call<ExternalScanResult>('ScanVersionDirectory', path),
  searchMinecraftFolders: (root: string) =>
    call<ExternalScanResult[]>('SearchMinecraftFolders', root),
  detectLocalLaunchers: () => call<ExternalScanResult[]>('DetectLocalLaunchers'),
  importExternalInstance: (root: string, candidateId: string, name: string) =>
    call<ExternalImportResponse>('ImportExternalInstance', root, candidateId, name),
  pickCustomSourceDirectory: () => call<string>('PickCustomSourceDirectory'),
  importCustomInstance: (request: CustomImportRequest) =>
    call<ExternalImportResponse>('ImportCustomInstance', request),

  getJavaRuntimes: () => call<JavaRuntime[]>('GetJavaRuntimes'),
  getJavaReleases: () => call<JavaReleaseInfo>('GetJavaReleases'),
  getJavaPackage: (feature: number) => call<JavaPackage>('GetJavaPackage', feature),
  installJava: (feature: number) => call<JavaRuntime>('InstallJava', feature),

  listServerProfiles: () => call<ServerProfile[]>('ListServerProfiles'),
  saveServerProfile: (request: ServerProfileRequest) =>
    call<ServerProfile>('SaveServerProfile', request),
  deleteServerProfile: (id: string) => call<void>('DeleteServerProfile', id),
  listMcsmInstances: (request: McsmConnectRequest) =>
    call<RemoteMcsmInstance[]>('ListMcsmInstances', request),
  getServerStatuses: () => call<ServerStatus[]>('GetServerStatuses'),
  startServer: (id: string) => call<void>('StartServer', id),
  stopServer: (id: string) => call<void>('StopServer', id),
  restartServer: (id: string) => call<void>('RestartServer', id),
  syncServerProfile: (id: string) => call<ServerSyncResult>('SyncServerProfile', id),
  previewServerSync: (id: string) => call<ServerSyncFile[]>('PreviewServerSync', id),
  syncServerProfileSelected: (id: string, files: string[]) =>
    call<ServerSyncResult>('SyncServerProfileSelected', id, files),
  getServerBackups: (id: string) => call<ServerBackup[]>('GetServerBackups', id),
  rollbackServer: (id: string, backupId: string) => call<void>('RollbackServer', id, backupId),
  syncLauncherProfiles: () => call<void>('SyncLauncherProfiles'),

  startLanDiscovery: (instanceId: string, broadcast: boolean) =>
    call<LanSnapshot>('StartLanDiscovery', instanceId, broadcast),
  stopLanDiscovery: () => call<void>('StopLanDiscovery'),
  startLanShare: (instanceId: string, dirs: string[]) =>
    call<LanSnapshot>('StartLanShare', instanceId, dirs),
  stopLanShare: () => call<LanSnapshot>('StopLanShare'),
  previewLanSync: (peerId: string, targetInstanceId: string) =>
    call<LanSyncPreview>('PreviewLanSync', peerId, targetInstanceId),
  previewLanVersion: (peerId: string) =>
    call<LanVersionPreview>('PreviewLanVersion', peerId),
  installLanVersion: (peerId: string, expectedDigest: string) =>
    call<LanVersionInstallResult>('InstallLanVersion', peerId, expectedDigest),
  syncFromLanPeer: (peerId: string, targetInstanceId: string, expectedDigest: string) =>
    call<LanSyncResult>('SyncFromLanPeer', peerId, targetInstanceId, expectedDigest),
  setLanBroadcast: (enabled: boolean) => call<LanSnapshot>('SetLanBroadcast', enabled),
  getLanSnapshot: () => call<LanSnapshot>('GetLanSnapshot'),
  scanLanSubnets: () => call<LanSnapshot>('ScanLanSubnets'),
  getLanTools: () => call<LanToolState>('GetLanTools'),
  sendLanChat: (peerId: string, text: string) =>
    call<LanToolSendResult>('SendLanChat', peerId, text),
  sendLanFile: (peerId: string, path: string) =>
    call<LanToolSendResult>('SendLanFile', peerId, path),
  sendLanVoice: (peerId: string, data: string, mime: string, durationMs: number) =>
    call<LanToolSendResult>('SendLanVoice', peerId, data, mime, durationMs),
  pickLanFile: () => call<LanFileInfo>('PickLanFile'),
  getLanVoice: (id: string) => call<string>('GetLanVoice', id),
  openLanInbox: () => call<void>('OpenLanInbox'),
};
