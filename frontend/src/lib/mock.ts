import type {
  AccountState,
  AccountView,
  AuthlibServer,
  Bootstrap,
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
  LaunchState,
  ModpackImportResponse,
  Project,
  ProjectVersion,
  ResourceProject,
  ResourceSearchRequest,
  ResourceSearchResult,
  ResourceType,
  ResourceVersion,
  SearchRequest,
  SearchResult,
  ServerPackExportResult,
  Settings,
  UpdateInstanceRequest,
  VersionList,
  VersionView,
  YSSDeviceView,
} from './types';

type Listener = (payload: unknown) => void;
const bus = new Map<string, Set<Listener>>();

export function mockSubscribe(event: string, handler: Listener): () => void {
  const set = bus.get(event) ?? new Set<Listener>();
  set.add(handler);
  bus.set(event, set);
  return () => set.delete(handler);
}

function emit(event: string, payload: unknown) {
  bus.get(event)?.forEach((handler) => handler(payload));
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms));

function nowMinus(minutes: number) {
  return new Date(Date.now() - minutes * 60_000).toISOString();
}

const mockBase = 'D:/Games/MyGoLuncher';

const initialSettings: Settings = {
  playerName: 'Steve',
  defaultMemoryMB: 0,
  defaultJavaPath: '',
  curseForgeAPIKey: '',
  extraJvmArgs: '',
  downloadWorkers: 8,
  showGameConsole: true,
  showLauncherOnLaunch: false,
  hitokotoApiUrl: 'https://hitokotogo.yhdzz.cn/v2',
  languageAuto: true,
  language: 'zh-CN',
  versionIsolation: true,
  aiSearchEnabled: false,
  aiSearchProvider: 'deepseek',
  aiSearchBaseUrl: '',
  aiSearchApiKey: '',
  aiSearchModel: 'deepseek-chat',
  schemaVersion: 3,
  microsoftClientID: '',
  customMirrors: [],
};

const instanceSeed: Array<Partial<InstanceView> & { id: string; name: string; mcVersion: string; loader: string }> = [
  {
    id: 'aurora',
    name: '暮色森林 · 轻量生存',
    mcVersion: '1.21.1',
    loader: 'fabric',
    loaderVersion: '0.16.10',
    launchVersionId: 'fabric-loader-0.16.10-1.21.1',
    memoryMB: 6144,
    requiredJava: 21,
    modCount: 24,
    shaderCount: 3,
    resourcePackCount: 2,
    lastPlayed: nowMinus(78),
    createdAt: nowMinus(15000),
    playTimeMinutes: 1482,
    source: 'manual',
    versionInstalled: true,
  },
  {
    id: 'create',
    name: '机械动力 · 创造工坊',
    mcVersion: '1.20.1',
    loader: 'forge',
    loaderVersion: '47.2.0',
    launchVersionId: '1.20.1-forge-47.2.0',
    memoryMB: 8192,
    requiredJava: 17,
    modCount: 61,
    shaderCount: 1,
    resourcePackCount: 4,
    lastPlayed: nowMinus(1520),
    createdAt: nowMinus(40000),
    playTimeMinutes: 5120,
    source: 'modpack',
    versionInstalled: true,
  },
  {
    id: 'vanilla',
    name: '原味生存',
    mcVersion: '26.3',
    loader: 'vanilla',
    loaderVersion: '',
    launchVersionId: '26.3',
    memoryMB: 4096,
    requiredJava: 25,
    modCount: 0,
    shaderCount: 0,
    resourcePackCount: 0,
    lastPlayed: nowMinus(7800),
    createdAt: nowMinus(9000),
    playTimeMinutes: 320,
    source: 'manual',
    versionInstalled: true,
  },
];

let instances: InstanceView[] = instanceSeed.map((seed) => ({
  gameDir: `${mockBase}/.minecraft/instances/${seed.id}`,
  javaPath: '',
  hasJavaOverride: false,
  running: false,
  notes: '',
  versionInstalled: true,
  modCount: 0,
  shaderCount: 0,
  resourcePackCount: 0,
  playTimeMinutes: 0,
  source: 'manual',
  requiredJava: 21,
  loaderVersion: '',
  launchVersionId: seed.mcVersion,
  memoryMB: 4096,
  lastPlayed: nowMinus(1000),
  createdAt: nowMinus(10000),
  ...seed,
  autoMemory: (seed.memoryMB ?? 4096) === 0,
  effectiveMemoryMB: (seed.memoryMB ?? 4096) || 8192,
}));

let settings: Settings = { ...initialSettings };
const initialAccounts: AccountView[] = [
  { id: 'offline-steve', kind: 'offline', name: 'Steve', uuid: '', serverUrl: '', expiresAt: '', label: '离线' },
  { id: 'msa-demo', kind: 'microsoft', name: 'CraftPlayer', uuid: '069a79f444e94726a5befca90e38aaf5', serverUrl: '', expiresAt: '', label: 'Microsoft 正版' },
];
let accounts: AccountView[] = initialAccounts;
let selectedAccountId = 'offline-steve';
let yssCancelled = false;
const installedVersions = new Set(['1.21.1', '1.20.1', '26.3']);
const logs = new Map<string, string[]>();
const launchTimers = new Map<string, ReturnType<typeof setInterval>>();
let installedJava: JavaRuntime[] = [
  {
    path: 'C:/Program Files/Java/jre1.8.0_511/bin/java.exe',
    home: 'C:/Program Files/Java/jre1.8.0_511',
    version: '1.8.0_511',
    major: 8,
    vendor: 'Oracle',
    architecture: 'amd64',
    managed: false,
    source: 'system',
  },
  {
    path: 'D:/Games/MyGoLuncher/.minecraft/runtimes/temurin-21/bin/java.exe',
    home: 'D:/Games/MyGoLuncher/.minecraft/runtimes/temurin-21',
    version: '21.0.12',
    major: 21,
    vendor: 'Temurin',
    architecture: 'amd64',
    managed: true,
    source: 'managed',
  },
  {
    path: 'D:/Games/MyGoLuncher/.minecraft/runtimes/temurin-25/bin/java.exe',
    home: 'D:/Games/MyGoLuncher/.minecraft/runtimes/temurin-25',
    version: '25.0.2',
    major: 25,
    vendor: 'Temurin',
    architecture: 'amd64',
    managed: true,
    source: 'managed',
  },
];

const paths = {
  base: mockBase,
  root: `${mockBase}/MyGoLuncher`,
  data: `${mockBase}/.minecraft`,
  instances: `${mockBase}/.minecraft/instances`,
  versions: `${mockBase}/.minecraft/versions`,
  libraries: `${mockBase}/.minecraft/libraries`,
  assets: `${mockBase}/.minecraft/assets`,
  runtimes: `${mockBase}/.minecraft/runtimes`,
  cache: `${mockBase}/MyGoLuncher/cache`,
};

const versionSeeds: Array<[string, VersionView['type'], string]> = [
  ['26.3', 'release', '2026-09-15T11:23:02Z'],
  ['26.4-snapshot-5', 'snapshot', '2026-10-20T12:15:06Z'],
  ['26.4-snapshot-4', 'snapshot', '2026-10-13T12:05:46Z'],
  ['26.2', 'release', '2026-06-17T10:20:00Z'],
  ['26.1.2', 'release', '2026-04-22T09:10:00Z'],
  ['26.1', 'release', '2026-02-11T08:30:00Z'],
  ['1.21.11', 'release', '2025-12-09T10:00:00Z'],
  ['1.21.10', 'release', '2025-10-21T11:00:00Z'],
  ['1.21.9', 'release', '2025-09-30T09:00:00Z'],
  ['1.21.8', 'release', '2025-07-17T12:00:00Z'],
  ['1.21.7', 'release', '2025-06-30T10:00:00Z'],
  ['1.21.6', 'release', '2025-06-17T11:00:00Z'],
  ['1.21.5', 'release', '2025-03-25T10:00:00Z'],
  ['1.21.4', 'release', '2024-12-03T10:00:00Z'],
  ['1.21.1', 'release', '2024-08-08T10:00:00Z'],
  ['1.21', 'release', '2024-06-13T10:00:00Z'],
  ['1.20.6', 'release', '2024-04-29T10:00:00Z'],
  ['1.20.4', 'release', '2023-12-07T10:00:00Z'],
  ['1.20.1', 'release', '2023-06-12T10:00:00Z'],
  ['1.19.4', 'release', '2023-03-14T10:00:00Z'],
  ['1.18.2', 'release', '2022-02-28T10:00:00Z'],
  ['1.17.1', 'release', '2021-07-06T10:00:00Z'],
  ['1.16.5', 'release', '2021-01-14T10:00:00Z'],
  ['1.12.2', 'release', '2017-09-18T10:00:00Z'],
  ['1.8.9', 'release', '2015-12-09T10:00:00Z'],
  ['1.7.10', 'release', '2014-06-26T10:00:00Z'],
  ['b1.7.3', 'old_beta', '2011-07-08T10:00:00Z'],
  ['a1.2.6', 'old_alpha', '2010-12-02T10:00:00Z'],
  ['24w14a', 'snapshot', '2024-04-03T10:00:00Z'],
  ['23w13a', 'snapshot', '2023-03-29T10:00:00Z'],
];

const javaMajorFor = (id: string) => {
  if (!id.startsWith('1.')) return 25;
  const parts = id.split('.');
  const minor = Number(parts[1] ?? 0);
  const patch = Number((parts[2] ?? '0').replace(/\D.*$/, '')) || 0;
  if (minor >= 21) return 21;
  if (minor === 20 && patch >= 5) return 21;
  if (minor >= 18) return 17;
  if (minor === 17) return 16;
  return 8;
};

function versionViews(): VersionView[] {
  return versionSeeds.map(([id, type, releaseTime]) => ({
    id,
    type,
    releaseTime,
    javaMajor: javaMajorFor(id),
    installed: installedVersions.has(id),
  }));
}

const projects: Record<string, Project[]> = {
  mod: [
    {
      project_id: 'AANobbMI',
      project_type: 'mod',
      slug: 'sodium',
      title: 'Sodium',
      description: 'A high-performance rendering engine replacement for Minecraft, which greatly improves frame rates and reduces micro-stutter.',
      author: 'jellysquid3',
      categories: ['fabric', 'neoforge', 'optimization'],
      downloads: 252857104,
      follows: 42086,
      icon_url: 'https://cdn.modrinth.com/data/AANobbMI/295862f4724dc3f78df3447ad6072b2dcd3ef0c9_96.webp',
      versions: ['1.21.1', '1.20.1', '1.19.4'],
      latest_version: 'g6ju16VJ',
      color: 8703084,
    },
    {
      project_id: 'gvQqBUqZ',
      project_type: 'mod',
      slug: 'lithium',
      title: 'Lithium',
      description: 'No-compromises game logic optimization mod. It works on the client and server, and improves tick performance.',
      author: 'jellysquid3',
      categories: ['fabric', 'neoforge', 'optimization'],
      downloads: 142097494,
      follows: 24310,
      icon_url: 'https://cdn.modrinth.com/data/gvQqBUqZ/bcc8686c13af0143adf4285d741256af824f70b7_96.webp',
      versions: ['1.21.1', '1.20.1', '1.19.4'],
      latest_version: 'lithium-1',
      color: 5088742,
    },
    {
      project_id: 'LNytGWDc',
      project_type: 'mod',
      slug: 'create',
      title: 'Create',
      description: 'Building tools and aesthetic technology. Create lets you automate nearly everything with kinetic machinery.',
      author: 'simibubi',
      categories: ['forge', 'neoforge', 'technology'],
      downloads: 28215501,
      follows: 19520,
      icon_url: 'https://cdn.modrinth.com/data/LNytGWDc/61d716699bcf1ec42ed4926a9e1c7311be6087e2_96.webp',
      versions: ['1.20.1', '1.19.2'],
      latest_version: 'create-1',
      color: 12161272,
    },
    {
      project_id: 'P7dR8mSH',
      project_type: 'mod',
      slug: 'fabric-api',
      title: 'Fabric API',
      description: 'Lightweight and modular API providing common hooks and intercompatibility measures utilized by mods using the Fabric toolchain.',
      author: 'modmuss50',
      categories: ['fabric', 'library'],
      downloads: 288323801,
      follows: 39020,
      icon_url: 'https://cdn.modrinth.com/data/P7dR8mSH/icon.png',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'fabric-api-1',
      color: 15608532,
    },
    {
      project_id: 'u6dRKJwZ',
      project_type: 'mod',
      slug: 'jei',
      title: 'Just Enough Items (JEI)',
      description: 'View items and recipes for all your favorite mods. A clean and intuitive item and recipe viewer.',
      author: 'mezz',
      categories: ['fabric', 'forge', 'neoforge', 'utility'],
      downloads: 86004365,
      follows: 13002,
      icon_url: 'https://cdn.modrinth.com/data/u6dRKJwZ/4a3f18ac0d096c9f8e9176984c44be4e58f94c89_96.webp',
      versions: ['1.21.1', '1.20.1', '1.19.4'],
      latest_version: 'jei-1',
      color: 15773604,
    },
    {
      project_id: 'uXXizFIs',
      project_type: 'mod',
      slug: 'ferrite-core',
      title: 'FerriteCore',
      description: 'Memory usage optimization mod. Reduces the memory footprint of Minecraft without changing behavior.',
      author: 'malte0811',
      categories: ['fabric', 'forge', 'optimization'],
      downloads: 166493935,
      follows: 16890,
      icon_url: 'https://cdn.modrinth.com/data/uXXizFIs/222a126f26f8f9ae1eb339f3b767677f18bff31f_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'ferrite-1',
      color: 2500134,
    },
    {
      project_id: 'YL57xq9U',
      project_type: 'mod',
      slug: 'iris',
      title: 'Iris Shaders',
      description: 'A modern shader pack loader for Minecraft. Designed for compatibility and performance with Sodium.',
      author: 'coderbot',
      categories: ['fabric', 'neoforge', 'optimization'],
      downloads: 196039734,
      follows: 28112,
      icon_url: 'https://cdn.modrinth.com/data/YL57xq9U/18d0e7f076d3d6ed5bedd472b853909aac5da202_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'iris-1',
      color: 2631720,
    },
    {
      project_id: '1bokaNcj',
      project_type: 'mod',
      slug: 'xaeros-minimap',
      title: "Xaero's Minimap",
      description: 'A feature-rich minimap with waypoints, entity radar and a beautiful world map.',
      author: 'xaero96',
      categories: ['fabric', 'forge', 'neoforge', 'utility'],
      downloads: 121091529,
      follows: 15402,
      icon_url: 'https://cdn.modrinth.com/data/1bokaNcj/354080f65407e49f486fcf9c4580e82c45ae63b8_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'xaero-1',
      color: 12418800,
    },
  ],
  shader: [
    {
      project_id: 'HVnmMxH1',
      project_type: 'shader',
      slug: 'complementary-reimagined',
      title: 'Complementary Shaders - Reimagined',
      description: 'Preserving the elements of Minecraft with exceptional quality, detail, and performance. A complete visual overhaul.',
      author: 'EminGT',
      categories: ['shaders', 'realistic'],
      downloads: 72723568,
      follows: 22410,
      icon_url: 'https://cdn.modrinth.com/data/HVnmMxH1/79cb7c8123bbc54945305b2ebad6b8881efdf5f8_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'comp-1',
      color: 16752748,
    },
    {
      project_id: 'R6NEzAwj',
      project_type: 'shader',
      slug: 'complementary-unbound',
      title: 'Complementary Shaders - Unbound',
      description: 'Transforming the visuals of Minecraft with exceptional quality, detail, and performance.',
      author: 'EminGT',
      categories: ['shaders', 'fantasy'],
      downloads: 47161071,
      follows: 15092,
      icon_url: 'https://cdn.modrinth.com/data/R6NEzAwj/c85ce4049aac76360d2cd24fd9a7003de01ef312_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'unbound-1',
      color: 14761786,
    },
    {
      project_id: 'Q1vvjJYV',
      project_type: 'shader',
      slug: 'bsl-shaders',
      title: 'BSL Shaders',
      description: "Shaderpack for Minecraft: Java Edition. It's bright, colorful, and distinct.",
      author: 'CaptTatsu',
      categories: ['shaders', 'colorful'],
      downloads: 31848313,
      follows: 10922,
      icon_url: 'https://cdn.modrinth.com/data/Q1vvjJYV/2a611a3cb434fb52fb81fa5dace13c5d8b67e55d_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'bsl-1',
      color: 4697880,
    },
    {
      project_id: 'lLqFfGNs',
      project_type: 'shader',
      slug: 'photon-shader',
      title: 'Photon Shaders',
      description: 'A gameplay-focused shader pack with a semi-realistic style, soft lighting and volumetric clouds.',
      author: 'sixthsurge',
      categories: ['shaders', 'realistic'],
      downloads: 29440930,
      follows: 8012,
      icon_url: 'https://cdn.modrinth.com/data/lLqFfGNs/39cb5f12e7dcc68d6cb666f225fcb2b801dd70fb_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'photon-1',
      color: 3114540,
    },
    {
      project_id: 'EpQFjzrQ',
      project_type: 'shader',
      slug: 'solas-shader',
      title: 'Solas Shader',
      description: 'A modern fantasy stylized shaderpack with colored lighting and stunning visuals.',
      author: 'Septonious',
      categories: ['shaders', 'fantasy'],
      downloads: 18742799,
      follows: 4920,
      icon_url: 'https://cdn.modrinth.com/data/EpQFjzrQ/e3efc6ba7a63f9e1cf473a794d0224a6daf243c7_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'solas-1',
      color: 12329282,
    },
    {
      project_id: 'ZvMtQlho',
      project_type: 'shader',
      slug: 'bliss-shader',
      title: 'Bliss Shaders',
      description: 'A well performing fantasy styled shaderpack with emphasis on scene variation and atmosphere.',
      author: 'Xonk',
      categories: ['shaders', 'fantasy'],
      downloads: 15866026,
      follows: 3870,
      icon_url: 'https://cdn.modrinth.com/data/ZvMtQlho/90145c971ea24387775108fc86c89bed9bd2c8f1_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'bliss-1',
      color: 9265227,
    },
    {
      project_id: 'izsIPI7a',
      project_type: 'shader',
      slug: 'makeup-ultra-fast',
      title: 'MakeUp - Ultra Fast',
      description: 'A shaderpack focused on the best quality / performance ratio, with a clean and colorful look.',
      author: 'KDXavier',
      categories: ['shaders', 'performance'],
      downloads: 14216693,
      follows: 3260,
      icon_url: 'https://cdn.modrinth.com/data/izsIPI7a/a08432baa86b8ffd58c08f4b3a001ef976ff764d_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'makeup-1',
      color: 8822789,
    },
    {
      project_id: 'kmwfVOoi',
      project_type: 'shader',
      slug: 'rethinking-voxels',
      title: 'Rethinking Voxels',
      description: 'A gameplay-focused voxel shaderpack with coloured lighting, realistic shadows and atmospheric fog.',
      author: 'gri573',
      categories: ['shaders', 'realistic'],
      downloads: 13830859,
      follows: 5120,
      icon_url: 'https://cdn.modrinth.com/data/kmwfVOoi/fc89eadad417dd376b14c3b31e1a2b87acaca034_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'voxels-1',
      color: 2553647,
    },
  ],
  resourcepack: [
    {
      project_id: '50dA9Sha',
      project_type: 'resourcepack',
      slug: 'fresh-animations',
      title: 'Fresh Animations',
      description: 'Make your game look like the trailers! Dynamic animated entities to freshen your Minecraft experience.',
      author: 'FreshLX',
      categories: ['entities', 'tweaks'],
      downloads: 51756381,
      follows: 21041,
      icon_url: 'https://cdn.modrinth.com/data/50dA9Sha/3132c10e9e3c73fde9799720fd3da5561071708c_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'fresh-1',
      color: 16097604,
    },
    {
      project_id: 'yfDziwn1',
      project_type: 'resourcepack',
      slug: 'translations-for-sodium',
      title: 'Translations for Sodium',
      description: 'Unofficial translations for the Sodium Minecraft mod, covering dozens of languages.',
      author: 'robotkoer',
      categories: ['translations', 'tweaks'],
      downloads: 23314959,
      follows: 2140,
      icon_url: 'https://cdn.modrinth.com/data/yfDziwn1/907581019df45903df237952ce8d10ac37134cb5_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'trans-1',
      color: 3137789,
    },
    {
      project_id: 'uvpymuxq',
      project_type: 'resourcepack',
      slug: 'better-leaves',
      title: "Motschen's Better Leaves",
      description: 'Improves the appearance of leaves with high mod compatibility and performance!',
      author: 'Motschen',
      categories: ['blocks', 'tweaks'],
      downloads: 21686247,
      follows: 3920,
      icon_url: 'https://cdn.modrinth.com/data/uvpymuxq/fe1a61998ae57dc6ad1a4bb028334c3c3925d22f_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'leaves-1',
      color: 3448617,
    },
    {
      project_id: 'slufHzC2',
      project_type: 'resourcepack',
      slug: 'fresh-moves',
      title: 'Fresh Moves',
      description: 'Fancy player animations that work with the EMF player animation mod.',
      author: 'IthanMendoza',
      categories: ['entities', 'animated'],
      downloads: 15657638,
      follows: 2710,
      icon_url: 'https://cdn.modrinth.com/data/slufHzC2/02b78db6655ad8dd6cfd847f99c76f7552c053ec_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'moves-1',
      color: 7500152,
    },
    {
      project_id: 'dspVZXKP',
      project_type: 'resourcepack',
      slug: 'fast-better-grass',
      title: 'Fast Better Grass',
      description: 'Makes grass use the top texture on the sides. Compatible with Sodium and other performance mods.',
      author: 'robotkoer',
      categories: ['blocks', 'performance'],
      downloads: 14079369,
      follows: 1680,
      icon_url: 'https://cdn.modrinth.com/data/dspVZXKP/5de0774f423d220fcd4e635b91f2bdfcb8a2b910.png',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'grass-1',
      color: 5439244,
    },
    {
      project_id: 'ItHr72Fy',
      project_type: 'resourcepack',
      slug: 'fullbright-ub',
      title: 'Fullbright UB',
      description: 'Experience fullbright in Sodium, OptiFine and vanilla with this simple resource pack.',
      author: 'worldresourcepack',
      categories: ['utility', 'tweaks'],
      downloads: 12406002,
      follows: 1520,
      icon_url: 'https://cdn.modrinth.com/data/ItHr72Fy/c28c5e4ae4e1e5ac726ff192855d16d2c15c83e2_96.webp',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'bright-1',
      color: 13092348,
    },
    {
      project_id: 'RRxvWKNC',
      project_type: 'resourcepack',
      slug: 'low-on-fire',
      title: 'Low On Fire',
      description: 'Lowers the fire overlay so it does not obstruct your view while burning.',
      author: 'Haikis',
      categories: ['utility', 'tweaks'],
      downloads: 17573937,
      follows: 2410,
      icon_url: 'https://cdn.modrinth.com/data/RRxvWKNC/06a7e7691a6da41798c255108a8563d7ffea171d.png',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'fire-1',
      color: 14243846,
    },
    {
      project_id: 'tN4E9NfV',
      project_type: 'resourcepack',
      slug: 'chat-reporting-helper',
      title: 'Chat Reporting Helper',
      description: 'An educational tool that explains chat reporting in a simple and neutral way.',
      author: 'robotkoer',
      categories: ['utility', 'tweaks'],
      downloads: 18136003,
      follows: 1980,
      icon_url: 'https://cdn.modrinth.com/data/tN4E9NfV/c2d93fd85469a512f67e53baa7648c1abe6645ef.png',
      versions: ['1.21.1', '1.20.1'],
      latest_version: 'chat-1',
      color: 13942723,
    },
  ],
};

function progressTask(title: string, messages: string[], totalMs = 1500): Promise<void> {
  const taskId = `mock-${Date.now()}`;
  const total = 100;
  return new Promise((resolve) => {
    let current = 0;
    emit('task:progress', {
      taskId, title, stage: 'prepare', message: messages[0] ?? '准备中…',
      current: 0, total, percent: 0, done: false, failed: false,
    });
    const timer = setInterval(() => {
      current = Math.min(100, current + 7 + Math.random() * 12);
      const idx = Math.min(messages.length - 1, Math.floor((current / 100) * messages.length));
      emit('task:progress', {
        taskId, title, stage: 'progress', message: messages[idx], current,
        total, percent: current, done: false, failed: false,
      });
      if (current >= 100) {
        clearInterval(timer);
        emit('task:progress', {
          taskId, title, stage: 'done', message: '操作完成', current: 1, total: 1,
          percent: 100, done: true, failed: false,
        });
        setTimeout(resolve, 180);
      }
    }, Math.max(50, totalMs / 14));
  });
}

function clone<T>(value: T): T {
  return structuredClone(value);
}

function bootstrap(): Bootstrap {
  return {
    settings: clone(settings),
    instances: clone(instances),
    paths,
    version: '1.0.0',
    initError: '',
    running: instances.filter((i) => i.running).map((i) => i.id),
    accounts: clone(accounts),
    selectedAccountId,
    totalMemoryMB: 65536,
    autoMemoryMB: 8192,
  };
}

function projectVersions(projectID: string): ProjectVersion[] {
  const project = Object.values(projects).flat().find((p) => p.project_id === projectID || p.slug === projectID);
  if (!project) return [];
  const suffix = project.project_type === 'shader' ? 'shaderpack.zip' : project.project_type === 'resourcepack' ? 'resourcepack.zip' : `${project.slug}.jar`;
  return project.versions.slice(0, 4).map((gameVersion, index) => ({
    id: `${project.project_id}-v${index + 1}`,
    project_id: project.project_id,
    name: `${project.title} ${gameVersion} · ${index === 0 ? 'Release' : 'Stable'}`,
    version_number: `mc${gameVersion}-1.${index + 2}.0`,
    game_versions: [gameVersion],
    loaders: project.project_type === 'mod' ? ['fabric', 'forge'] : [],
    date_published: nowMinus(900 + index * 7000),
    version_type: index === 0 ? 'release' : 'beta',
    downloads: Math.round(project.downloads / (index + 4)),
    files: [{
      url: 'https://example.invalid/mock-download',
      filename: `${project.slug}-${gameVersion}-${suffix}`,
      primary: true,
      size: 2_400_000 - index * 120_000,
      hashes: {},
    }],
    dependencies: [],
  }));
}


function makeResource(
  source: 'modrinth' | 'curseforge',
  type: ResourceType,
  projectId: string,
  slug: string,
  title: string,
  author: string,
  description: string,
  downloads: number,
  categories: string[],
  gameVersions: string[],
  iconUrl = '',
): ResourceProject {
  return {
    id: `${source}:${projectId}`,
    projectId,
    source,
    type,
    slug,
    title,
    description,
    author,
    iconUrl,
    websiteUrl: source === 'modrinth'
      ? `https://modrinth.com/${type}/${slug}`
      : `https://www.curseforge.com/minecraft/${type === 'modpack' ? 'modpacks' : type === 'resourcepack' ? 'texture-packs' : type === 'datapack' ? 'data-packs' : type === 'shader' ? 'shaders' : 'mc-mods'}/${slug}`,
    downloads,
    categories,
    gameVersions,
    latestVersion: '1.0.0',
    color: 0,
    dateModified: nowMinus(1440),
  };
}

function mockResourceCatalog(): ResourceProject[] {
  const out: ResourceProject[] = [];
  for (const type of ['mod', 'shader', 'resourcepack'] as const) {
    for (const project of projects[type] ?? []) {
      out.push({
        id: `modrinth:${project.project_id}`,
        projectId: project.project_id,
        source: 'modrinth',
        type,
        slug: project.slug,
        title: project.title,
        description: project.description,
        author: project.author,
        iconUrl: project.icon_url,
        websiteUrl: `https://modrinth.com/${type}/${project.slug}`,
        downloads: project.downloads,
        categories: project.categories,
        gameVersions: project.versions,
        latestVersion: project.latest_version,
        color: project.color,
        dateModified: nowMinus(2000),
      });
    }
  }
  out.push(
    makeResource('modrinth', 'datapack', 'OhduvhIc', 'veinminer', 'VeinMiner', 'ChocoTaco', 'Mine an entire ore vein with a single click. Works as a server-friendly data pack.', 4820000, ['utility', 'datapacks'], ['1.21.1', '1.20.1']),
    makeResource('modrinth', 'datapack', '8oi3bsk5', 'terralith', 'Terralith', 'Starmute', 'A world generation overhaul that adds over 95 new biomes while remaining compatible with vanilla terrain.', 19300000, ['worldgen', 'adventure'], ['1.21.1', '1.20.1']),
    makeResource('modrinth', 'datapack', '9s9hQ6JR', 'incendium', 'Incendium', 'Starmute', 'Transforms the Nether with new biomes, structures, and challenging exploration.', 7910000, ['worldgen', 'adventure'], ['1.21.1', '1.20.1']),
    makeResource('modrinth', 'modpack', '1KVo5zza', 'fabulously-optimized', 'Fabulously Optimized', 'robotkoer', 'A simple, performance-focused modpack with Sodium, Iris and the best quality-of-life mods.', 10800000, ['fabric', 'optimization'], ['1.21.1', '1.20.1']),
    makeResource('modrinth', 'modpack', 'l9m9tuPN', 'zombie-invade-100-days', 'Zombie Invade 100 Days', 'MCPackTeam', 'Survive escalating zombie hordes for 100 in-game days in this adventure modpack.', 4200000, ['adventure', 'challenge'], ['1.20.1', '1.19.2']),
    makeResource('curseforge', 'mod', '60089', 'mouse-tweaks', 'Mouse Tweaks', 'YaLTeR', 'Enhances inventory management with drag scrolling and quick item moving.', 48200000, ['fabric', 'forge', 'utility'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'mod', '248787', 'appleskin', 'AppleSkin', 'squeek502', 'Adds food and saturation information to tooltips and the HUD.', 37200000, ['fabric', 'forge', 'utility'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'mod', '388172', 'geckolib', 'GeckoLib', 'GeckoTheGecko', 'A 3D animation engine used by many popular mods.', 42100000, ['fabric', 'forge', 'library'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'mod', '32274', 'journeymap', 'JourneyMap', 'techbrew', 'Real-time minimap and full-screen world map with waypoints.', 29600000, ['fabric', 'forge', 'utility'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'shader', '385587', 'complementary-unbound', 'Complementary Shaders - Unbound', 'EminGT', 'High quality, highly customizable shader pack built for visual fidelity and performance.', 47100000, ['shaders', 'fantasy'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'shader', '235329', 'sildurs-vibrant-shaders', "Sildur's Vibrant Shaders", 'Sildur', 'A colorful shader pack with strong lighting, shadows and water effects.', 31800000, ['shaders', 'colorful'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'shader', '293195', 'seus-renewed', 'SEUS Renewed', 'Sonic Ether', 'A classic realistic shader pack with soft lighting and atmospheric effects.', 19400000, ['shaders', 'realistic'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'resourcepack', '226375', 'xray-ultimate', 'Xray Ultimate', 'AmbientOcclusion', 'A utility resource pack that makes valuable ores easy to see.', 40300000, ['utility', 'tweaks'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'resourcepack', '297657', 'stay-true', 'Stay True', 'FredBob', 'A vanilla-friendly resource pack that keeps the original atmosphere while improving detail.', 12800000, ['vanilla-like', 'tweaks'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'resourcepack', '456977', 'better-vanilla-building', 'Better Vanilla Building', 'Sterling', 'Improves building blocks with connected textures and depth.', 8700000, ['blocks', 'tweaks'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'datapack', '1331777', 'mes-biomes-o-plenty-compat', 'MES - Biomes O Plenty Compat Pack', 'MES Team', 'Compatibility data pack that adds more structures and recipes for Biomes O Plenty.', 1480000, ['datapacks', 'worldgen'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'datapack', '811803', 'spawn-animations-datapack', 'Spawn Animations [Data Pack]', 'Tschipp', 'Adds a dramatic spawn animation when players join a world.', 2190000, ['datapacks', 'utility'], ['1.21.1', '1.20.1']),
    makeResource('curseforge', 'modpack', '285109', 'rlcraft', 'RLCraft', 'Shivaxi', 'A hardcore survival modpack where everything wants you dead.', 68700000, ['forge', 'adventure', 'challenge'], ['1.12.2']),
    makeResource('curseforge', 'modpack', '925200', 'all-the-mods-10', 'All the Mods 10 - ATM10', 'ATMTeam', 'A massive kitchen-sink modpack for NeoForge with hundreds of mods and quests.', 31200000, ['neoforge', 'technology', 'magic'], ['1.21.1']),
    makeResource('curseforge', 'modpack', '331758', 'better-mc', 'Better MC [FORGE] BMC4', 'Luna Pixel Studios', 'A curated modpack focused on exploration, combat, and beautiful world generation.', 26400000, ['forge', 'adventure', 'optimization'], ['1.20.1']),
    makeResource('curseforge', 'modpack', '389001', 'the-pixelmon-modpack', 'The Pixelmon Modpack', 'Pixelmon Team', 'Catch, train and battle Pokémon in Minecraft with the Pixelmon mod.', 19800000, ['forge', 'adventure', 'magic'], ['1.20.1']),
  );
  return out;
}

function mockResourceVersions(instanceId: string, source: string, projectId: string, resourceType: ResourceType): ResourceVersion[] {
  const catalog = mockResourceCatalog();
  const project = catalog.find((item) => item.source === source && item.projectId === projectId && item.type === resourceType);
  if (!project) return [];
  const target = instances.find((item) => item.id === instanceId);
  if (resourceType !== 'modpack' && !target) throw new Error('请先选择一个版本');
  if (resourceType === 'mod' && target?.loader === 'vanilla') throw new Error('原版游戏无法加载模组');
  const baseGame = resourceType === 'modpack'
    ? (project.gameVersions.length ? project.gameVersions : ['1.21.1'])
    : [target!.mcVersion];
  const loaders = resourceType === 'mod' ? [target!.loader] : [];
  const now = Date.now();
  return Array.from({ length: Math.min(4, Math.max(1, baseGame.length + 1)) }).map((_, index) => {
    const gameVersion = baseGame[index % baseGame.length];
    const extension = resourceType === 'shader' || resourceType === 'resourcepack' || resourceType === 'datapack' || resourceType === 'modpack' ? 'zip' : 'jar';
    return {
      id: `${source}-${projectId}-file-${index + 1}`,
      projectId,
      source: source as 'modrinth' | 'curseforge',
      name: `${project.title} ${gameVersion} ${index === 0 ? 'Release' : 'Stable'}`,
      versionNumber: `${gameVersion}-1.${index + 2}.0`,
      releaseType: index === 0 ? 'release' : 'beta',
      datePublished: new Date(now - (index + 1) * 86400000 * 7).toISOString(),
      gameVersions: [gameVersion],
      loaders,
      downloads: Math.round(project.downloads / (index + 5)),
      fileName: `${project.slug}-${gameVersion}.${extension}`,
      fileSize: 2_400_000 - index * 160_000,
      downloadable: true,
    };
  });
}

function mockCreateModpackInstance(title: string, mcVersion: string, loader: string): InstanceView {
  const id = `pack-${Date.now()}`;
  const view: InstanceView = {
    id, name: title, mcVersion, loader,
    loaderVersion: loader === 'forge' ? '47.2.0' : loader === 'neoforge' ? '21.1.77' : loader === 'quilt' ? '0.26.4' : '0.16.10',
    launchVersionId: `${loader}-${mcVersion}-mock`,
    memoryMB: 6144, javaPath: '', createdAt: new Date().toISOString(), lastPlayed: '',
    playTimeMinutes: 0, source: 'modpack', notes: '从资源中心安装',
    gameDir: `${paths.instances}/${id}`, requiredJava: javaMajorFor(mcVersion), versionInstalled: true,
    running: false, modCount: 87, shaderCount: 1, resourcePackCount: 2, hasJavaOverride: false,
    autoMemory: false, effectiveMemoryMB: 6144,
  };
  instances = [view, ...instances];
  emit('instances:changed', null);
  return view;
}

export async function mockCall<T>(method: string, args: unknown[]): Promise<T> {
  switch (method) {
    case 'Bootstrap':
      return clone(bootstrap()) as T;
    case 'GetInstances':
      return clone(instances) as T;
    case 'TestDownloadMirrors':
      await sleep(900);
      return [
        { id: 'official', name: 'Mojang 官方', url: 'https://piston-meta.mojang.com/...', ok: true, latencyMs: 420, bytes: 278138, bytesPerSec: 662233 },
        { id: 'bmclapi', name: 'BMCLAPI', url: 'https://bmclapi2.bangbang93.com/...', ok: true, latencyMs: 680, bytes: 278138, bytesPerSec: 409026 },
        ...(args[0] as Array<{ id: string; name: string; url: string }> ?? []).map((item) => ({
          id: item.id, name: item.name, url: item.url, ok: true, latencyMs: 910, bytes: 278138, bytesPerSec: 305640,
        })),
      ] as T;
    case 'GetAccounts':
      return { accounts: clone(accounts), selectedId: selectedAccountId } satisfies AccountState as T;
    case 'SelectAccount': {
      const id = String(args[0]);
      if (!accounts.some((account) => account.id === id)) throw new Error('账户不存在');
      selectedAccountId = id;
      emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
      return { accounts: clone(accounts), selectedId: selectedAccountId } satisfies AccountState as T;
    }
    case 'AddOfflineAccount': {
      const name = String(args[0]).trim() || 'Steve';
      let account = accounts.find((item) => item.kind === 'offline' && item.name.toLowerCase() === name.toLowerCase());
      if (!account) {
        account = { id: `offline-${Date.now()}`, kind: 'offline', name, uuid: '', serverUrl: '', expiresAt: '', label: '离线' };
        accounts = [...accounts, account];
      }
      selectedAccountId = account.id;
      settings.playerName = name;
      emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
      return { accounts: clone(accounts), selectedId: selectedAccountId } satisfies AccountState as T;
    }
    case 'RenameOfflineAccount': {
      const id = String(args[0]);
      const name = String(args[1]).trim() || 'Steve';
      const account = accounts.find((item) => item.id === id && item.kind === 'offline');
      if (!account) throw new Error('离线账户不存在');
      if (accounts.some((item) => item.kind === 'offline' && item.id !== id && item.name.toLowerCase() === name.toLowerCase())) {
        throw new Error('已存在同名离线账户');
      }
      account.name = name;
      selectedAccountId = id;
      settings.playerName = name;
      emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
      return { accounts: clone(accounts), selectedId: selectedAccountId } satisfies AccountState as T;
    }
    case 'RemoveAccount': {
      if (accounts.length <= 1) throw new Error('至少需要保留一个账户');
      const id = String(args[0]);
      accounts = accounts.filter((account) => account.id !== id);
      if (selectedAccountId === id) selectedAccountId = accounts[0]?.id ?? '';
      emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
      return { accounts: clone(accounts), selectedId: selectedAccountId } satisfies AccountState as T;
    }
    case 'LoginAuthlib': {
      await sleep(700);
      const serverUrl = String(args[0]);
      const username = String(args[1]);
      const account: AccountView = {
        id: `authlib-${Date.now()}`, kind: 'authlib', name: username,
        uuid: '00000000000000000000000000000001', serverUrl,
        expiresAt: '', label: '外置登录',
      };
      accounts = [account, ...accounts.filter((item) => item.kind !== 'authlib' || item.name !== username)];
      selectedAccountId = account.id;
      emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
      return clone(account) as T;
    }
    case 'BeginMicrosoftLogin': {
      window.setTimeout(() => {
        const account: AccountView = {
          id: `msa-${Date.now()}`, kind: 'microsoft', name: 'CraftPlayer',
          uuid: '069a79f444e94726a5befca90e38aaf5', serverUrl: '',
          expiresAt: new Date(Date.now() + 86400000).toISOString(), label: 'Microsoft 正版',
        };
        accounts = [account, ...accounts.filter((item) => item.kind !== 'microsoft' || item.name !== 'CraftPlayer')];
        selectedAccountId = account.id;
        emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
        emit('auth:login', { success: true, message: 'Microsoft 登录成功', account: clone(account) });
      }, 2200);
      return {
        userCode: 'ABCD-EFGH', verificationUri: 'https://www.microsoft.com/link',
        expiresIn: 900, interval: 5,
        message: '在浏览器中打开链接并输入代码完成登录。',
      } satisfies DeviceCodeView as T;
    }
    case 'CancelMicrosoftLogin':
      return undefined as T;
    case 'BeginYSSLogin': {
      const serverUrl = String(args[0] ?? 'https://yss.1v.fit');
      yssCancelled = false;
      window.setTimeout(() => {
        if (yssCancelled) return;
        const account: AccountView = {
          id: `yss-${Date.now()}`, kind: 'authlib', name: 'Steve',
          uuid: '069a79f444e94726a5befca90e38aaf5', serverUrl,
          expiresAt: '', label: '外置登录',
        };
        accounts = [account, ...accounts.filter((item) => item.kind !== 'authlib' || item.name !== 'Steve')];
        selectedAccountId = account.id;
        emit('accounts:changed', { accounts: clone(accounts), selectedId: selectedAccountId });
        emit('auth:yss', { success: true, message: 'YSS 授权成功', account: clone(account) });
      }, 2400);
      return {
        userCode: 'ABCD-EFGH',
        verificationUri: 'https://yss.1v.fit/oauth/device',
        verificationUriComplete: 'https://yss.1v.fit/oauth/device?user_code=ABCD-EFGH',
        expiresIn: 600,
        interval: 5,
        message: '',
      } satisfies YSSDeviceView as T;
    }
    case 'CancelYSSLogin':
      yssCancelled = true;
      return undefined as T;
    case 'SaveSettings': {
      settings = { ...(args[0] as Settings) };
      return clone(settings) as T;
    }
    case 'GetHitokoto': {
      await sleep(400);
      const quotes: Hitokoto[] = [
        { text: '生活不止眼前的苟且，还应该有诗和远方的田野。', type: 'g', from: '生活不止眼前的苟且', fromWho: '许巍' },
        { text: '永远年轻，永远热泪盈眶。', type: 'g', from: 'The Beat Generation', fromWho: '杰克·凯鲁亚克' },
        { text: '世界上只有一种真正的英雄主义，那就是认清生活的真相后依然热爱它。', type: 'g', from: '米开朗基罗传', fromWho: '罗曼·罗兰' },
      ];
      return clone(quotes[Math.floor(Math.random() * quotes.length)]) as T;
    }
    case 'ListVersions': {
      const includeSnapshots = Boolean(args[0]);
      const includeOld = Boolean(args[1]);
      const filtered = versionViews().filter((v) =>
        v.type === 'release' ||
        (includeSnapshots && v.type === 'snapshot') ||
        (includeOld && (v.type === 'old_beta' || v.type === 'old_alpha')));
      return { versions: filtered, latestRelease: '26.3', latestSnapshot: '26.4-snapshot-5' } satisfies VersionList as T;
    }
    case 'InstallVersion': {
      const id = String(args[0]);
      await progressTask(`安装 Minecraft ${id}`, ['正在准备版本元数据…', '正在下载游戏本体…', '正在下载运行库…', '正在同步游戏资源…', '正在写入版本配置…'], 2200);
      installedVersions.add(id);
      return undefined as T;
    }
    case 'CreateInstance': {
      const req = args[0] as CreateInstanceRequest;
      await progressTask(`创建版本 ${req.name || req.mcVersion}`, ['正在准备游戏文件…', '正在安装运行库…', '正在配置加载器…', '正在创建游戏目录…'], 2000);
      const id = `instance-${Date.now()}`;
      installedVersions.add(req.mcVersion);
      const view: InstanceView = {
        id, name: req.name || `${req.mcVersion} 版本`, mcVersion: req.mcVersion,
        loader: req.loader, loaderVersion: req.loaderVersion || (req.loader === 'fabric' ? '0.16.10' : ''),
        launchVersionId: req.mcVersion, memoryMB: req.memoryMB || settings.defaultMemoryMB,
        javaPath: '', createdAt: new Date().toISOString(), lastPlayed: '',
        playTimeMinutes: 0, source: 'manual', notes: '',
        gameDir: `${paths.instances}/${id}`, requiredJava: javaMajorFor(req.mcVersion),
        versionInstalled: true, running: false, modCount: 0, shaderCount: 0,
        resourcePackCount: 0, hasJavaOverride: false,
        autoMemory: !req.memoryMB, effectiveMemoryMB: req.memoryMB || 8192,
      };
      instances = [view, ...instances];
      emit('instances:changed', null);
      return clone(view) as T;
    }
    case 'UpdateInstance': {
      const req = args[0] as UpdateInstanceRequest;
      const idx = instances.findIndex((i) => i.id === req.id);
      if (idx >= 0) {
        instances[idx] = {
          ...instances[idx], name: req.name, memoryMB: req.memoryMB,
          javaPath: req.javaPath, hasJavaOverride: Boolean(req.javaPath),
          autoMemory: req.memoryMB === 0, effectiveMemoryMB: req.memoryMB || 8192,
        };
        emit('instances:changed', null);
        return clone(instances[idx]) as T;
      }
      throw new Error('版本不存在');
    }
    case 'DeleteInstance': {
      const id = String(args[0]);
      instances = instances.filter((i) => i.id !== id);
      emit('instances:changed', null);
      return undefined as T;
    }
    case 'LaunchInstance': {
      const id = String(args[0]);
      const instance = instances.find((i) => i.id === id);
      if (!instance) throw new Error('版本不存在');
      instance.running = true;
      logs.set(id, ['[启动器] 正在启动 ' + instance.name + ' · Java ' + instance.requiredJava, '[启动器] 正在校验游戏文件…']);
      emit('launch:state', { instanceId: id, running: true, exitCode: 0, durationSeconds: 0, stoppedByUser: false } satisfies LaunchState);
      emit('instances:changed', null);
      const lines = [
        '[Render thread/INFO] Setting user: ' + settings.playerName,
        '[Render thread/INFO] Backend library: LWJGL version 3.4.1+1',
        '[Render thread/INFO] Reloading ResourceManager: vanilla',
        '[Render thread/INFO] Created: 1024x512 minecraft:textures/atlas/blocks.png',
        '[Render thread/INFO] Sound engine started',
        '[Render thread/INFO] OpenAL initialized',
        '[Render thread/INFO] Environment: Minecraft Java Edition',
      ];
      let index = 0;
      const timer = setInterval(() => {
        if (index >= lines.length) { clearInterval(timer); launchTimers.delete(id); return; }
        logs.get(id)?.push(lines[index]);
        emit('launch:log', { instanceId: id, line: lines[index] });
        index++;
      }, 900);
      launchTimers.set(id, timer);
      return undefined as T;
    }
    case 'StopInstance': {
      const id = String(args[0]);
      const instance = instances.find((i) => i.id === id);
      if (instance) instance.running = false;
      const timer = launchTimers.get(id);
      if (timer) clearInterval(timer);
      launchTimers.delete(id);
      emit('launch:state', { instanceId: id, running: false, exitCode: 0, durationSeconds: 780, stoppedByUser: true } satisfies LaunchState);
      emit('instances:changed', null);
      return undefined as T;
    }
    case 'GetInstanceLog':
      return clone(logs.get(String(args[0])) ?? []) as T;
    case 'OpenInstanceFolder':
    case 'OpenDataFolder':
    case 'OpenConfigFolder':
    case 'OpenURL':
      return undefined as T;
    case 'SearchProjects': {
      const req = args[0] as SearchRequest;
      const type = req.projectType || 'mod';
      const query = req.query.trim().toLowerCase();
      let hits = projects[type] ?? [];
      if (query) {
        hits = hits.filter((p) => `${p.title} ${p.description} ${p.author}`.toLowerCase().includes(query));
      }
      if (req.gameVersion && req.gameVersion !== 'all') {
        hits = hits.filter((p) => !p.versions.length || p.versions.includes(req.gameVersion));
      }
      await sleep(180);
      return { hits: clone(hits), total_hits: hits.length, offset: req.offset, limit: req.limit } as T;
    }

    case 'SearchResources': {
      const req = args[0] as ResourceSearchRequest;
      const target = instances.find((item) => item.id === req.instanceId);
      const gameVersion = req.type === 'modpack' ? '' : (target?.mcVersion ?? '');
      const loader = req.type === 'mod' ? (target?.loader ?? '') : '';
      const cjkPattern = /[㐀-鿿぀-ヿ가-힯]/;
      const aliases: Array<[string, string]> = [
        ['苹果皮', 'appleskin'], ['旅行地图', 'journeymap'], ['小地图', 'minimap'],
        ['光影', 'shader'], ['材质包', 'resource pack'], ['资源包', 'resource pack'],
        ['优化', 'optimization'], ['钠', 'sodium'], ['锂', 'lithium'], ['铷', 'rubidium'],
      ];
      const rawQuery = req.query.trim();
      let queryUsed = rawQuery;
      for (const [from, to] of aliases) queryUsed = queryUsed.split(from).join(to);
      const hadCJK = cjkPattern.test(rawQuery);
      const aiUsed = Boolean(req.useAI && hadCJK);
      const cjkQuery = cjkPattern.test(queryUsed) && !aiUsed;
      let items = mockResourceCatalog().filter((item) => item.type === req.type);
      if (req.source !== 'all') items = items.filter((item) => item.source === req.source);
      if (queryUsed) {
        const query = queryUsed.toLowerCase();
        items = items.filter((item) => `${item.title} ${item.description} ${item.author}`.toLowerCase().includes(query));
      }
      if (req.type !== 'modpack') {
        items = items.filter((item) => item.gameVersions.length === 0 || item.gameVersions.includes(gameVersion));
      }
      if (req.type === 'mod' && loader && loader !== 'vanilla') {
        items = items.filter((item) => item.categories.length === 0 || item.categories.includes(loader));
      }
      const modrinth = items.filter((item) => item.source === 'modrinth');
      const curseforge = items.filter((item) => item.source === 'curseforge');
      let merged: ResourceProject[] = [];
      if (req.source === 'modrinth') merged = modrinth.slice(req.offset, req.offset + req.limit);
      else if (req.source === 'curseforge') merged = curseforge.slice(req.offset, req.offset + req.limit);
      else {
        const left = modrinth.slice(req.offset, req.offset + req.limit);
        const right = curseforge.slice(req.offset, req.offset + req.limit);
        for (let i = 0; i < Math.max(left.length, right.length); i++) {
          if (left[i]) merged.push(left[i]);
          if (right[i]) merged.push(right[i]);
        }
      }
      await sleep(300);
      return {
        items: clone(merged), total: modrinth.length + curseforge.length,
        offset: req.offset, limit: req.limit, warnings: [], curseForgeReady: true,
        queryUsed, aiUsed, aiError: '', cjkQuery,
      } satisfies ResourceSearchResult as T;
    }
    case 'ListResourceVersions': {
      await sleep(220);
      return clone(mockResourceVersions(
        String(args[0]), String(args[1]), String(args[2]), String(args[3]) as ResourceType,
      )) as T;
    }
    case 'InstallResource': {
      const projectId = String(args[2]);
      const source = String(args[1]);
      const item = mockResourceCatalog().find((entry) => entry.projectId === projectId && entry.source === source);
      await progressTask(`安装 ${item?.title ?? '资源'}`, ['正在自动匹配版本…', '正在下载文件…', '正在补全必需前置…', '正在写入版本目录…'], 1900);
      return {
        fileName: item ? `${item.slug}-auto.${item.type === 'shader' || item.type === 'resourcepack' || item.type === 'datapack' ? 'zip' : 'jar'}` : 'download.jar',
        path: '/mock/download.jar', versionId: 'auto', versionName: `${item?.title ?? '资源'} 最新兼容版`,
        gameVersion: instances.find((entry) => entry.id === String(args[0]))?.mcVersion ?? '1.21.1',
        loader: item?.type === 'mod' ? 'fabric' : '', dependencies: item?.slug === 'iris' ? ['Sodium'] : [],
      } satisfies InstallProjectResult as T;
    }
    case 'InstallResourceModpack': {
      const source = String(args[0]);
      const projectId = String(args[1]);
      const item = mockResourceCatalog().find((entry) => entry.projectId === projectId && entry.source === source && entry.type === 'modpack');
      await progressTask(`安装整合包 ${item?.title ?? ''}`, ['正在下载整合包…', '正在读取清单…', '正在下载模组文件…', '正在安装加载器…'], 2600);
      const view = mockCreateModpackInstance(item?.title ?? 'Imported Modpack', item?.gameVersions[0] ?? '1.20.1', 'fabric');
      return {
        instance: clone(view), format: source === 'modrinth' ? 'mrpack' : 'curseforge',
        formatLabel: source === 'modrinth' ? 'Modrinth (.mrpack)' : 'CurseForge (.zip)',
        missingFiles: 0, warnings: [],
      } satisfies ModpackImportResponse as T;
    }
    case 'ListProjectVersions': {
      await sleep(150);
      const instance = instances.find((item) => item.id === String(args[0]));
      const versions = projectVersions(String(args[1]));
      if (!instance) throw new Error('请先选择一个版本');
      if (versions[0]?.project_id && !instance?.mcVersion) return [] as T;
      return versions.filter((version) => {
        const project = Object.values(projects).flat().find((item) => item.project_id === version.project_id);
        const gameMatch = version.game_versions.includes(instance.mcVersion);
        if (!project) return gameMatch;
        if (project.project_type !== 'mod') return gameMatch;
        if (instance.loader === 'vanilla') return false;
        return gameMatch && version.loaders.includes(instance.loader);
      }) as T;
    }
    case 'InstallProject': {
      const project = Object.values(projects).flat().find((p) => p.project_id === args[1]);
      await progressTask(`安装 ${project?.title ?? '项目'}`, ['正在解析版本信息…', '正在下载文件…', '正在检查前置依赖…', '正在写入版本目录…'], 1600);
      const version = project ? projectVersions(project.project_id)[0] : undefined;
      return {
        fileName: project ? `${project.slug}-installed.jar` : 'download.jar',
        path: '/mock/download.jar',
        versionId: version?.id ?? '',
        versionName: version?.name ?? 'Latest',
        gameVersion: version?.game_versions[0] ?? '1.21.1',
        loader: version?.loaders[0] ?? '',
        dependencies: project?.slug === 'iris' ? ['Sodium'] : [],
      } satisfies InstallProjectResult as T;
    }
    case 'ImportModpackDialog':
    case 'InstallModpackProject': {
      await progressTask('导入整合包', ['正在读取整合包…', '正在展开配置覆盖…', '正在下载模组文件…', '正在安装 Fabric 加载器…'], 2600);
      const id = `pack-${Date.now()}`;
      const view: InstanceView = {
        id, name: 'Roguelike Adventures', mcVersion: '1.20.1', loader: 'fabric',
        loaderVersion: '0.16.10', launchVersionId: 'fabric-loader-0.16.10-1.20.1',
        memoryMB: 6144, javaPath: '', createdAt: new Date().toISOString(), lastPlayed: '',
        playTimeMinutes: 0, source: 'mrpack', notes: '从整合包导入',
        gameDir: `${paths.instances}/${id}`, requiredJava: 17, versionInstalled: true,
        running: false, modCount: 147, shaderCount: 2, resourcePackCount: 6, hasJavaOverride: false,
        autoMemory: false, effectiveMemoryMB: 6144,
      };
      instances = [view, ...instances];
      emit('instances:changed', null);
      return {
        instance: clone(view), format: 'mrpack', formatLabel: 'Modrinth (.mrpack)',
        missingFiles: 0, warnings: [],
      } satisfies ModpackImportResponse as T;
    }
    case 'ExportModpackDialog':
    case 'ExportInstanceBackupDialog':
      await progressTask('导出版本', ['正在收集版本文件…', '正在压缩资源与配置…', '正在写入整合包…'], 1200);
      return 'C:/Users/im/Desktop/export.mrpack' as T;
    case 'ExportServerPackDialog':
      await progressTask('导出服务器包', ['正在准备服务器运行时…', '正在打包模组与配置…', '正在写入启动脚本…'], 2200);
      return { path: 'C:/Users/im/Desktop/MyGoLuncher-server.zip', warnings: [] } satisfies ServerPackExportResult as T;
    case 'AuthlibServers':
      return [
        { id: 'littleskin', name: 'LittleSkin', url: 'https://littleskin.cn/api/yggdrasil' },
        { id: 'yss', name: 'YggdrasilSkinServer', url: 'https://yss.1v.fit' },
      ] satisfies AuthlibServer[] as T;
    case 'PickExternalLauncher':
    case 'PickVersionDirectory':
      await sleep(350);
      return {
        root: 'C:/Users/im/AppData/Roaming/.minecraft',
        source: '官方启动器',
        candidates: [
          { id: 'versions/fabric-1.21.1', name: 'fabric-1.21.1', mcVersion: '1.21.1', loader: 'fabric', loaderVersion: '0.16.10', source: '官方启动器', modCount: 37, hasSaves: true, warnings: [] },
          { id: 'versions/1.20.1-forge-47.2.0', name: '1.20.1-forge-47.2.0', mcVersion: '1.20.1', loader: 'forge', loaderVersion: '47.2.0', source: '官方启动器', modCount: 62, hasSaves: false, warnings: [] },
          { id: 'versions/1.19.2', name: '1.19.2', mcVersion: '1.19.2', loader: 'vanilla', loaderVersion: '', source: '官方启动器', modCount: 0, hasSaves: true, warnings: [] },
        ],
      } satisfies ExternalScanResult as T;
    case 'ScanVersionDirectory': {
      const root = String(args[0] ?? '');
      return {
        root, source: '其他启动器',
        candidates: [
          { id: 'version:1.21.1', name: '1.21.1', mcVersion: '1.21.1', loader: 'vanilla', loaderVersion: '', source: '其他启动器', modCount: 0, hasSaves: false, warnings: [] },
        ],
      } satisfies ExternalScanResult as T;
    }
    case 'DetectLocalLaunchers':
      await sleep(500);
      return [{
        root: 'C:/Users/im/AppData/Roaming/.minecraft',
        source: '官方启动器',
        candidates: [
          { id: 'versions/1.21.1', name: '1.21.1', mcVersion: '1.21.1', loader: 'vanilla', loaderVersion: '', source: '官方启动器', modCount: 0, hasSaves: true, warnings: [] },
          { id: 'versions/1.20.1-forge-47.2.0', name: '1.20.1-forge-47.2.0', mcVersion: '1.20.1', loader: 'forge', loaderVersion: '47.2.0', source: '官方启动器', modCount: 62, hasSaves: false, warnings: [] },
        ],
      }] satisfies ExternalScanResult[] as T;
    case 'ImportExternalInstance': {
      await progressTask('导入 HMCL 版本', ['正在复制存档与配置…', '正在复制模组…', '正在准备游戏文件…'], 2600);
      const id = `external-${Date.now()}`;
      const view: InstanceView = {
        id, name: '迁移的 Fabric 生存', mcVersion: '1.21.1', loader: 'fabric',
        loaderVersion: '0.16.10', launchVersionId: 'fabric-loader-0.16.10-1.21.1',
        memoryMB: 6144, javaPath: '', createdAt: new Date().toISOString(), lastPlayed: '',
        playTimeMinutes: 0, source: 'external:HMCL', notes: '从 HMCL 版本导入',
        gameDir: `${paths.instances}/${id}`, requiredJava: 21, versionInstalled: true,
        running: false, modCount: 37, shaderCount: 2, resourcePackCount: 3, hasJavaOverride: false,
        autoMemory: false, effectiveMemoryMB: 6144,
      };
      instances = [view, ...instances];
      emit('instances:changed', null);
      return {
        instance: clone(view), source: 'HMCL', filesCopied: 412, modsCopied: 37,
        resourcePacksCopied: 3, shaderPacksCopied: 2, savesCopied: 1, warnings: [],
      } satisfies ExternalImportResponse as T;
    }
    case 'GetJavaRuntimes':
      await sleep(500);
      return clone(installedJava) as T;
    case 'GetJavaReleases':
      return { available: [8, 11, 17, 21, 25, 26, 27], lts: [8, 11, 17, 21, 25], latestFeature: 27 } satisfies JavaReleaseInfo as T;
    case 'GetJavaPackage': {
      const feature = Number(args[0]);
      const known = [8, 11, 17, 21, 25, 26, 27];
      if (!known.includes(feature)) throw new Error('Adoptium 暂未提供该版本');
      return {
        feature,
        name: `OpenJDK${feature}U-jre_x64_windows_hotspot_${feature}.0.2_1.zip`,
        url: 'https://example.invalid/java.zip', sha256: '', size: 43_000_000 + feature * 200_000,
        version: `${feature}.0.2+9`, releaseName: `jdk-${feature}.0.2+9`, os: 'windows', arch: 'x64',
        installed: installedJava.some((r) => r.major === feature),
      } satisfies JavaPackage as T;
    }
    case 'InstallJava': {
      const feature = Number(args[0]);
      await progressTask(`安装 Java ${feature}`, ['正在下载 Temurin JRE…', '正在校验归档…', '正在解压运行时…', '正在检测 Java…'], 2400);
      if (!installedJava.some((r) => r.major === feature)) {
        installedJava = [{
          path: `${paths.runtimes}/temurin-${feature}/bin/java.exe`,
          home: `${paths.runtimes}/temurin-${feature}`,
          version: `${feature}.0.2`, major: feature, vendor: 'Temurin',
          architecture: 'amd64', managed: true, source: 'managed',
        }, ...installedJava];
      }
      const runtime = installedJava.find((r) => r.major === feature)!;
      if (!settings.defaultJavaPath) settings.defaultJavaPath = runtime.path;
      return clone(runtime) as T;
    }
    default:
      throw new Error(`演示模式不支持调用 ${method}`);
  }
}
