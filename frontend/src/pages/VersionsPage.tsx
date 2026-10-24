import { useEffect, useMemo, useState } from 'react';
import {
  ArrowDownToLine,
  Check,
  FolderInput,
  Laptop,
  LoaderCircle,
  RefreshCw,
  Search,
  TriangleAlert,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import {
  LocalVersionModal,
  buildLocalEntries,
} from '../components/LocalVersionModal';
import type { ExternalScanResult, VersionList, VersionView } from '../lib/types';
import { cn, formatDate } from '../lib/format';

type VersionFilter = 'release' | 'snapshot' | 'all';

const typeLabels: Record<string, string> = {
  release: '正式版',
  snapshot: '快照',
  old_beta: 'Beta',
  old_alpha: 'Alpha',
};

const filters: Array<{ key: VersionFilter; label: string }> = [
  { key: 'release', label: '正式版' },
  { key: 'snapshot', label: '快照' },
  { key: 'all', label: '全部' },
];

export function VersionsPage() {
  const {
    pushToast, refreshInstances, selectInstance, navigate,
  } = useApp();
  const [filter, setFilter] = useState<VersionFilter>('release');
  const [query, setQuery] = useState('');
  const [data, setData] = useState<VersionList | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [visible, setVisible] = useState(50);
  const [pending, setPending] = useState<Set<string>>(new Set());
  const [reloadKey, setReloadKey] = useState(0);

  const [scanning, setScanning] = useState(false);
  const [picking, setPicking] = useState(false);
  const [scans, setScans] = useState<ExternalScanResult[]>([]);
  const [scanOpen, setScanOpen] = useState(false);
  const [selectedKey, setSelectedKey] = useState('');
  const [importing, setImporting] = useState(false);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    setError('');
    api.listVersions(filter !== 'release', filter === 'all')
      .then((result) => {
        if (!alive) return;
        setData(result);
        setVisible(50);
      })
      .catch((reason: Error) => {
        if (alive) setError(reason.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => { alive = false; };
  }, [filter, reloadKey]);

  const filtered = useMemo(() => {
    if (!data) return [];
    let versions = data.versions;
    if (filter === 'release') versions = versions.filter((version) => version.type === 'release');
    if (filter === 'snapshot') versions = versions.filter((version) => version.type === 'snapshot');
    if (query.trim()) {
      const lower = query.trim().toLowerCase();
      versions = versions.filter((version) => version.id.toLowerCase().includes(lower));
    }
    return versions;
  }, [data, filter, query]);

  const entries = useMemo(() => buildLocalEntries(scans), [scans]);

  const showScans = (results: ExternalScanResult[]) => {
    const found = buildLocalEntries(results);
    if (found.length === 0) {
      pushToast('error', '没有发现版本', '请选择包含 versions 目录或单个版本文件夹的目录。');
      return;
    }
    setScans(results);
    setSelectedKey(found[0].key);
    setScanOpen(true);
  };

  const detectLaunchers = async () => {
    setScanning(true);
    try {
      const results = await api.detectLocalLaunchers();
      showScans(results);
    } catch (reason) {
      pushToast('error', '未找到本地启动器', (reason as Error).message);
    } finally {
      setScanning(false);
    }
  };

  const pickDirectory = async () => {
    setPicking(true);
    try {
      const result = await api.pickVersionDirectory();
      if (!result.root) return;
      showScans([result]);
    } catch (reason) {
      pushToast('error', '目录扫描失败', (reason as Error).message);
    } finally {
      setPicking(false);
    }
  };

  const importEntry = async () => {
    const entry = entries.find((item) => item.key === selectedKey);
    if (!entry) return;
    setImporting(true);
    try {
      const imported = await api.importExternalInstance(entry.root, entry.candidate.id, entry.candidate.name);
      await refreshInstances();
      selectInstance(imported.instance.id);
      setScanOpen(false);
      const warning = imported.warnings.length > 0 ? `；${imported.warnings[0]}` : '';
      pushToast(
        'success',
        '版本已添加',
        `${imported.instance.name} 已加入启动器${warning}`,
      );
      navigate('home');
    } catch (reason) {
      pushToast('error', '版本导入失败', (reason as Error).message);
    } finally {
      setImporting(false);
    }
  };

  const install = async (version: VersionView) => {
    setPending((current) => new Set(current).add(version.id));
    try {
      await api.installVersion(version.id);
      setData((current) => current ? {
        ...current,
        versions: current.versions.map((entry) => entry.id === version.id ? { ...entry, installed: true } : entry),
      } : current);
      pushToast('success', '版本安装完成', `Minecraft ${version.id} 已准备好。`);
    } catch (reason) {
      pushToast('error', '版本安装失败', (reason as Error).message);
    } finally {
      setPending((current) => {
        const next = new Set(current);
        next.delete(version.id);
        return next;
      });
    }
  };

  return (
    <div className="page versions-page">
      <PageHeader
        kicker="VERSIONS"
        title={<>版本 <span className="title-accent">/</span> 添加与下载</>}
        description="添加官方启动器、其他启动器或任意目录中的版本，也可以直接下载 Mojang 官方版本。"
      />

      <section className="panel local-panel">
        <div className="panel-head">
          <div>
            <h2>添加本地版本</h2>
            <p>自动扫描本机官方启动器、HMCL、PCL2、Prism / MultiMC，或手动选择任意版本目录。</p>
          </div>
        </div>
        <div className="local-actions">
          <button type="button" className="btn btn-primary" onClick={() => void detectLaunchers()} disabled={scanning || picking}>
            {scanning ? <LoaderCircle size={16} className="spin" /> : <Laptop size={16} />}
            {scanning ? '正在扫描本机启动器…' : '扫描本机启动器'}
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => void pickDirectory()} disabled={scanning || picking}>
            {picking ? <LoaderCircle size={16} className="spin" /> : <FolderInput size={16} />}
            {picking ? '正在读取目录…' : '选择版本目录'}
          </button>
        </div>
        <p className="panel-note">
          支持选择 <code>.minecraft</code>、<code>versions</code> 文件夹或单个版本文件夹。导入时会复制存档、模组与配置，并重新准备游戏文件。
        </p>
      </section>

      <section className="panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2>下载 Minecraft</h2>
            <p>从 Mojang 官方源下载游戏本体、运行库与资源文件。</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((value) => value + 1)}>
            <RefreshCw size={14} className={loading ? 'spin' : ''} /> 刷新
          </button>
        </div>

        {error && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={17} />
            <div>
              <strong>版本清单加载失败</strong>
              <span>{error}</span>
            </div>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((value) => value + 1)}>重试</button>
          </div>
        )}

        <div className="toolbar">
          <div className="segmented">
            {filters.map((item) => (
              <button
                key={item.key}
                type="button"
                className={cn('segment', filter === item.key && 'is-active')}
                onClick={() => setFilter(item.key)}
              >
                {item.label}
              </button>
            ))}
          </div>
          <div className="search-field">
            <Search size={15} />
            <input
              value={query}
              onChange={(event) => {
                setQuery(event.target.value);
                setVisible(50);
              }}
              placeholder="搜索版本号"
              aria-label="搜索版本号"
            />
          </div>
          <span className="toolbar-count">{filtered.length} 个版本</span>
        </div>

        <div className="version-table">
          <div className="version-table-head">
            <span>版本</span>
            <span>类型</span>
            <span>发布时间</span>
            <span>Java</span>
            <span className="align-right">操作</span>
          </div>
          {loading ? (
            Array.from({ length: 6 }).map((_, index) => (
              <div className="version-row" key={index}>
                <span className="skeleton skeleton-line wide" />
                <span className="skeleton skeleton-pill" />
                <span className="skeleton skeleton-line" />
                <span className="skeleton skeleton-pill" />
                <span className="skeleton skeleton-button" />
              </div>
            ))
          ) : filtered.length === 0 ? (
            <EmptyState
              compact
              icon={<Search size={24} />}
              title="没有找到版本"
              body="换一个关键词，或切换正式版 / 快照筛选。"
            />
          ) : (
            filtered.slice(0, visible).map((version) => (
              <div className="version-row" key={version.id}>
                <span className="version-id"><strong>{version.id}</strong></span>
                <span>
                  <span className={`version-type type-${version.type}`}>{typeLabels[version.type] ?? version.type}</span>
                </span>
                <span className="version-date">{formatDate(version.releaseTime)}</span>
                <span className="version-java">Java {version.javaMajor}</span>
                <span className="version-action">
                  {version.installed ? (
                    <span className="installed-state"><Check size={14} /> 已安装</span>
                  ) : (
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => void install(version)}
                      disabled={pending.has(version.id)}
                    >
                      {pending.has(version.id) ? <LoaderCircle size={14} className="spin" /> : <ArrowDownToLine size={14} />}
                      {pending.has(version.id) ? '安装中' : '安装'}
                    </button>
                  )}
                </span>
              </div>
            ))
          )}
          {!loading && visible < filtered.length && (
            <button type="button" className="load-more" onClick={() => setVisible((value) => value + 50)}>
              加载更多版本 <span>{visible} / {filtered.length}</span>
            </button>
          )}
        </div>
      </section>

      <LocalVersionModal
        open={scanOpen}
        scans={scans}
        entries={entries}
        selectedKey={selectedKey}
        importing={importing}
        onSelect={setSelectedKey}
        onClose={() => setScanOpen(false)}
        onImport={() => void importEntry()}
      />
    </div>
  );
}
