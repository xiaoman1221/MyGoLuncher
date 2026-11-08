import { useEffect, useMemo, useState } from 'react';
import {
  ArrowDownToLine,
  Check,
  FolderInput,
  Laptop,
  LoaderCircle,
  Plus,
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
import { CustomVersionModal } from '../components/CustomVersionModal';
import type {
  ExternalImportResponse,
  ExternalScanResult,
  VersionList,
  VersionView,
} from '../lib/types';
import { cn, formatDate } from '../lib/format';

type VersionFilter = 'release' | 'snapshot' | 'all';

const typeKeys = {
  release: 'version.type.release',
  snapshot: 'version.type.snapshot',
  old_beta: 'version.type.old_beta',
  old_alpha: 'version.type.old_alpha',
} as const;

const filters: Array<{ key: VersionFilter; labelKey: 'versions.filter.release' | 'versions.filter.snapshot' | 'versions.filter.all' }> = [
  { key: 'release', labelKey: 'versions.filter.release' },
  { key: 'snapshot', labelKey: 'versions.filter.snapshot' },
  { key: 'all', labelKey: 'versions.filter.all' },
];

export function VersionsPage() {
  const {
    pushToast, refreshInstances, selectInstance, navigate, t, locale,
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
  const [selectedKeys, setSelectedKeys] = useState<Set<string>>(new Set());
  const [importing, setImporting] = useState(false);
  const [customOpen, setCustomOpen] = useState(false);

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

  const typeLabel = (type: string) => {
    const key = typeKeys[type as keyof typeof typeKeys];
    return key ? t(key) : type;
  };

  const showScans = (results: ExternalScanResult[]) => {
    const found = buildLocalEntries(results);
    if (found.length === 0) {
      pushToast('error', t('versions.noVersionsFound'), t('versions.noVersionsHint'));
      return;
    }
    setScans(results);
    setSelectedKeys(new Set([found[0].key]));
    setScanOpen(true);
  };

  const toggleKey = (key: string) => {
    setSelectedKeys((current) => {
      const next = new Set(current);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const selectAllEntries = () => setSelectedKeys(new Set(entries.map((entry) => entry.key)));
  const clearSelection = () => setSelectedKeys(new Set());

  const detectLaunchers = async () => {
    setScanning(true);
    try {
      const results = await api.detectLocalLaunchers();
      showScans(results);
    } catch (reason) {
      pushToast('error', t('versions.noLauncher'), (reason as Error).message);
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
      pushToast('error', t('versions.scanFailed'), (reason as Error).message);
    } finally {
      setPicking(false);
    }
  };

  const importEntries = async () => {
    const selected = entries.filter((entry) => selectedKeys.has(entry.key));
    if (selected.length === 0) return;
    setImporting(true);
    let success = 0;
    let lastInstanceID = '';
    const errors: string[] = [];
    const warnings: string[] = [];
    for (const entry of selected) {
      try {
        const imported = await api.importExternalInstance(entry.root, entry.candidate.id, entry.candidate.name);
        success += 1;
        lastInstanceID = imported.instance.id;
        if (imported.warnings.length > 0) {
          warnings.push(`${entry.candidate.name}: ${imported.warnings[0]}`);
        }
      } catch (reason) {
        errors.push(`${entry.candidate.name}: ${(reason as Error).message}`);
      }
    }
    await refreshInstances();
    if (lastInstanceID) selectInstance(lastInstanceID);
    setScanOpen(false);
    if (success > 0) {
      const summary = t('local.importSummary', { success, failed: errors.length });
      pushToast('success', t('versions.added'), warnings[0] ? `${summary} · ${warnings[0]}` : summary);
      navigate('home');
    }
    if (errors.length > 0) {
      pushToast('error', t('local.importPartial'), errors[0]);
    }
    setImporting(false);
  };

  const handleCustomImported = async (response: ExternalImportResponse) => {
    await refreshInstances();
    selectInstance(response.instance.id);
    setCustomOpen(false);
    const warning = response.warnings.length > 0 ? `；${response.warnings[0]}` : '';
    pushToast(
      'success',
      t('versions.added'),
      t('versions.addedDetail', { name: response.instance.name, warning }),
    );
    navigate('home');
  };

  const install = async (version: VersionView) => {
    setPending((current) => new Set(current).add(version.id));
    try {
      await api.installVersion(version.id);
      setData((current) => current ? {
        ...current,
        versions: current.versions.map((entry) => entry.id === version.id ? { ...entry, installed: true } : entry),
      } : current);
      // Re-fetch from the backend so the installed state is authoritative and
      // other rows (snapshots, shared versions) are refreshed too.
      setReloadKey((value) => value + 1);
      pushToast('success', t('versions.installDone'), t('versions.installDoneDetail', { id: version.id }));
    } catch (reason) {
      pushToast('error', t('versions.installFailed'), (reason as Error).message);
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
        title={<>{t('versions.title')} <span className="title-accent">/</span> {t('versions.titleTail')}</>}
        description={t('versions.description')}
      />

      <section className="panel local-panel">
        <div className="panel-head">
          <div>
            <h2>{t('versions.addLocalTitle')}</h2>
            <p>{t('versions.addLocalBody')}</p>
          </div>
        </div>
        <div className="local-actions">
          <button type="button" className="btn btn-primary" onClick={() => void detectLaunchers()} disabled={scanning || picking}>
            {scanning ? <LoaderCircle size={16} className="spin" /> : <Laptop size={16} />}
            {scanning ? t('versions.scanning') : t('versions.scanLaunchers')}
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => void pickDirectory()} disabled={scanning || picking}>
            {picking ? <LoaderCircle size={16} className="spin" /> : <FolderInput size={16} />}
            {picking ? t('versions.readingDirectory') : t('versions.pickDirectory')}
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => setCustomOpen(true)} disabled={scanning || picking || importing}>
            <Plus size={16} />
            {t('local.customOpen')}
          </button>
        </div>
        <p className="panel-note">{t('versions.localHint')}</p>
        <p className="panel-note">{t('versions.importCompat')}</p>
      </section>

      <section className="panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2>{t('versions.downloadTitle')}</h2>
            <p>{t('versions.downloadBody')}</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((value) => value + 1)}>
            <RefreshCw size={14} className={loading ? 'spin' : ''} /> {t('common.refresh')}
          </button>
        </div>

        {error && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={17} />
            <div>
              <strong>{t('versions.listFailed')}</strong>
              <span>{error}</span>
            </div>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setReloadKey((value) => value + 1)}>{t('common.retry')}</button>
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
                {t(item.labelKey)}
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
              placeholder={t('versions.searchPlaceholder')}
              aria-label={t('versions.searchPlaceholder')}
            />
          </div>
          <span className="toolbar-count">{t('versions.count', { count: filtered.length })}</span>
        </div>

        <div className="version-table">
          <div className="version-table-head">
            <span>{t('versions.tableVersion')}</span>
            <span>{t('versions.tableType')}</span>
            <span>{t('versions.tableReleased')}</span>
            <span>{t('versions.tableJava')}</span>
            <span className="align-right">{t('versions.tableActions')}</span>
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
              title={t('versions.emptyTitle')}
              body={t('versions.emptyBody')}
            />
          ) : (
            filtered.slice(0, visible).map((version) => (
              <div className="version-row" key={version.id}>
                <span className="version-id"><strong>{version.id}</strong></span>
                <span>
                  <span className={`version-type type-${version.type}`}>{typeLabel(version.type)}</span>
                </span>
                <span className="version-date">{formatDate(version.releaseTime, locale)}</span>
                <span className="version-java">Java {version.javaMajor}</span>
                <span className="version-action">
                  {version.installed ? (
                    <span className="installed-state"><Check size={14} /> {t('common.installed')}</span>
                  ) : (
                    <button
                      type="button"
                      className="btn btn-ghost btn-sm"
                      onClick={() => void install(version)}
                      disabled={pending.has(version.id)}
                    >
                      {pending.has(version.id) ? <LoaderCircle size={14} className="spin" /> : <ArrowDownToLine size={14} />}
                      {pending.has(version.id) ? t('common.installing') : t('common.install')}
                    </button>
                  )}
                </span>
              </div>
            ))
          )}
          {!loading && visible < filtered.length && (
            <button type="button" className="load-more" onClick={() => setVisible((value) => value + 50)}>
              {t('versions.loadMore')} <span>{visible} / {filtered.length}</span>
            </button>
          )}
        </div>
      </section>

      <LocalVersionModal
        open={scanOpen}
        scans={scans}
        entries={entries}
        selectedKeys={selectedKeys}
        importing={importing}
        onToggle={toggleKey}
        onSelectAll={selectAllEntries}
        onClearSelection={clearSelection}
        onClose={() => setScanOpen(false)}
        onImport={() => void importEntries()}
      />

      <CustomVersionModal
        open={customOpen}
        onClose={() => setCustomOpen(false)}
        onImported={handleCustomImported}
      />
    </div>
  );
}
