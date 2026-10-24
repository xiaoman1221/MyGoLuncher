import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  Files,
  History,
  KeyRound,
  LoaderCircle,
  Pencil,
  Play,
  Plus,
  RefreshCw,
  RotateCw,
  RotateCcw,
  Server,
  Square,
  Trash2,
  UploadCloud,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { Modal } from '../components/Modal';
import type {
  InstanceView,
  RemoteMcsmInstance,
  ServerBackup,
  ServerProfile,
  ServerProfileRequest,
  ServerStatus,
  ServerSyncDir,
  ServerSyncFile,
  ServerSyncResult,
} from '../lib/types';
import type { MessageKey } from '../lib/i18n';
import { cn } from '../lib/format';

const ALL_DIRS: ServerSyncDir[] = [
  'mods', 'config', 'defaultconfigs', 'scripts', 'kubejs', 'datapacks', 'resourcepacks', 'shaderpacks',
];

const DIR_KEYS: Record<ServerSyncDir, MessageKey> = {
  mods: 'servers.dir.mods',
  config: 'servers.dir.config',
  defaultconfigs: 'servers.dir.defaultconfigs',
  scripts: 'servers.dir.scripts',
  kubejs: 'servers.dir.kubejs',
  datapacks: 'servers.dir.datapacks',
  resourcepacks: 'servers.dir.resourcepacks',
  shaderpacks: 'servers.dir.shaderpacks',
};

function statusKey(status: ServerStatus | undefined): MessageKey {
  if (!status || status.error) return status ? 'servers.status.offline' : 'servers.status.unknown';
  switch (status.statusText) {
    case 'running': return 'servers.status.running';
    case 'stopped': return 'servers.status.stopped';
    case 'starting': return 'servers.status.starting';
    case 'stopping': return 'servers.status.stopping';
    case 'busy': return 'servers.status.busy';
    default: return 'servers.status.unknown';
  }
}

export function ServersPage() {
  const { instances, pushToast, t, locale } = useApp();
  const [profiles, setProfiles] = useState<ServerProfile[]>([]);
  const [statuses, setStatuses] = useState<Record<string, ServerStatus>>({});
  const [loading, setLoading] = useState(true);
  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<ServerProfile | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [syncTarget, setSyncTarget] = useState<ServerProfile | null>(null);
  const [historyTarget, setHistoryTarget] = useState<ServerProfile | null>(null);
  const statusBusy = useRef(false);

  const instanceName = useCallback((id: string) => (
    instances.find((instance) => instance.id === id)?.name ?? id
  ), [instances]);

  const loadStatuses = useCallback(async (silent = false) => {
    if (statusBusy.current) return;
    statusBusy.current = true;
    try {
      const rows = await api.getServerStatuses();
      setStatuses(Object.fromEntries(rows.map((row) => [row.profileId, row])));
    } catch (error) {
      if (!silent) pushToast('error', t('servers.connectionError'), (error as Error).message);
    } finally {
      statusBusy.current = false;
    }
  }, [pushToast, t]);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      setProfiles(await api.listServerProfiles());
    } catch (error) {
      pushToast('error', t('servers.saveFailed'), (error as Error).message);
    } finally {
      setLoading(false);
    }
  }, [pushToast, t]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (profiles.length > 0) void loadStatuses();
  }, [profiles.length, loadStatuses]);

  useEffect(() => {
    if (profiles.length === 0) return;
    const timer = window.setInterval(() => { void loadStatuses(true); }, 15_000);
    return () => window.clearInterval(timer);
  }, [profiles.length, loadStatuses]);

  const openAdd = () => {
    setEditing(null);
    setFormOpen(true);
  };

  const openEdit = (profile: ServerProfile) => {
    setEditing(profile);
    setFormOpen(true);
  };

  const runAction = async (id: string, action: 'start' | 'stop' | 'restart') => {
    setBusy(`${action}:${id}`);
    try {
      await (action === 'start' ? api.startServer(id) : action === 'stop' ? api.stopServer(id) : api.restartServer(id));
      await loadStatuses();
    } catch (error) {
      pushToast('error', t('servers.actionFailed'), (error as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const remove = async (profile: ServerProfile) => {
    if (!window.confirm(t('servers.deleteConfirm'))) return;
    setBusy(`delete:${profile.id}`);
    try {
      await api.deleteServerProfile(profile.id);
      await load();
    } catch (error) {
      pushToast('error', t('servers.saveFailed'), (error as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const formatTime = (value?: string) => {
    if (!value) return t('servers.neverSynced');
    const parsed = new Date(value);
    return Number.isNaN(parsed.getTime()) ? value : parsed.toLocaleString(locale);
  };

  return (
    <div className="page servers-page">
      <PageHeader
        kicker="SERVERS"
        title={<>{t('servers.title')} <span className="title-accent">/</span> MCSM</>}
        description={t('servers.description')}
        actions={
          <div className="header-actions">
            <button type="button" className="btn btn-ghost" onClick={() => void Promise.all([load(), loadStatuses()])} disabled={loading}>
              <RefreshCw size={15} className={loading ? 'spin' : undefined} />
              {t('common.refresh')}
            </button>
            <button type="button" className="btn btn-primary" onClick={openAdd} disabled={instances.length === 0}>
              <Plus size={16} />
              {t('servers.add')}
            </button>
          </div>
        }
      />

      {instances.length === 0 && (
        <div className="inline-banner banner-warning">
          <Server size={17} />
          <div>
            <strong>{t('servers.noLocalInstances')}</strong>
            <span>{t('servers.emptyBody')}</span>
          </div>
        </div>
      )}

      {loading && profiles.length === 0 ? (
        <div className="skeleton skeleton-line wide" />
      ) : profiles.length === 0 ? (
        <EmptyState
          icon={<Server size={24} />}
          title={t('servers.emptyTitle')}
          body={t('servers.emptyBody')}
          action={instances.length > 0 ? (
            <button type="button" className="btn btn-primary" onClick={openAdd}>
              <Plus size={16} /> {t('servers.add')}
            </button>
          ) : undefined}
        />
      ) : (
        <div className="server-grid">
          {profiles.map((profile) => {
            const status = statuses[profile.id];
            const online = Boolean(status?.online);
            const running = busy?.endsWith(`:${profile.id}`);
            return (
              <section key={profile.id} className="panel server-card">
                <div className="server-card-head">
                  <div className="server-card-title">
                    <span className={cn('server-dot', online ? 'is-online' : status?.error ? 'is-error' : 'is-idle')} />
                    <div>
                      <h2>{profile.name}</h2>
                      <p>{profile.panelUrl}</p>
                    </div>
                  </div>
                  <span className={cn('status-pill', online ? 'is-online' : status?.error ? 'is-error' : 'is-idle')}>
                    {t(statusKey(status))}
                  </span>
                </div>

                {status?.error && <div className="server-error">{status.error}</div>}
                <div className="server-meta">
                  <div>
                    <span>{t('servers.localInstance')}</span>
                    <strong>{instanceName(profile.localInstanceId)}</strong>
                  </div>
                  <div>
                    <span>{t('servers.remoteInstance')}</span>
                    <strong>{status?.nickname || profile.instanceUuid.slice(0, 8)}</strong>
                  </div>
                  <div>
                    <span>{t('servers.remoteRoot')}</span>
                    <strong className="mono">{profile.remoteRoot}</strong>
                  </div>
                  <div>
                    <span>{t('servers.lastSync', { time: '' })}</span>
                    <strong>{formatTime(profile.lastSync)}</strong>
                  </div>
                  {profile.lastRollback && (
                    <div>
                      <span>{t('servers.lastRollback', { time: '' })}</span>
                      <strong>{formatTime(profile.lastRollback)}</strong>
                    </div>
                  )}
                </div>

                <div className="server-dirs">
                  {profile.syncDirs.map((dir) => (
                    <span key={dir} className="chip">{t(DIR_KEYS[dir])}</span>
                  ))}
                  {profile.deleteExtras && <span className="chip chip-danger">{t('servers.deleteExtras')}</span>}
                  <span className={cn('chip', profile.apiKeySet ? 'is-key' : 'chip-warn')}>
                    <KeyRound size={12} />
                    {profile.apiKeySet ? t('servers.apiKeySet') : t('servers.apiKeyMissing')}
                  </span>
                </div>

                <div className="server-actions">
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    onClick={() => setSyncTarget(profile)}
                    disabled={running}
                  >
                    {running ? <LoaderCircle size={14} className="spin" /> : <UploadCloud size={14} />}
                    {running ? t('servers.syncing') : t('servers.sync')}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    onClick={() => setHistoryTarget(profile)}
                    disabled={running}
                  >
                    <History size={14} /> {t('servers.rollback')}
                  </button>
                  {online ? (
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => void runAction(profile.id, 'stop')} disabled={running}>
                      <Square size={13} /> {t('servers.stop')}
                    </button>
                  ) : (
                    <button type="button" className="btn btn-ghost btn-sm" onClick={() => void runAction(profile.id, 'start')} disabled={running}>
                      <Play size={14} /> {t('servers.start')}
                    </button>
                  )}
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void runAction(profile.id, 'restart')} disabled={running}>
                    <RotateCw size={14} /> {t('servers.restart')}
                  </button>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => openEdit(profile)} disabled={running}>
                    <Pencil size={14} /> {t('servers.edit')}
                  </button>
                  <button type="button" className="btn btn-danger-ghost btn-sm" onClick={() => void remove(profile)} disabled={running}>
                    <Trash2 size={14} /> {t('common.delete')}
                  </button>
                </div>
              </section>
            );
          })}
        </div>
      )}

      {formOpen && (
        <ServerForm
          key={editing?.id ?? 'new'}
          profile={editing}
          instances={instances}
          onClose={() => setFormOpen(false)}
          onSaved={async () => {
            setFormOpen(false);
            await load();
            await loadStatuses();
          }}
        />
      )}

      {syncTarget && (
        <ServerSyncModal
          profile={syncTarget}
          onClose={() => setSyncTarget(null)}
          onDone={async () => {
            setSyncTarget(null);
            await load();
            await loadStatuses();
          }}
        />
      )}

      {historyTarget && (
        <ServerRollbackModal
          profile={historyTarget}
          online={Boolean(statuses[historyTarget.id]?.online)}
          onClose={() => setHistoryTarget(null)}
          onDone={async () => {
            setHistoryTarget(null);
            await load();
            await loadStatuses();
          }}
        />
      )}
    </div>
  );
}

function formatBytes(value: number) {
  if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  if (value >= 1024) return `${(value / 1024).toFixed(0)} KB`;
  return `${value} B`;
}

function ServerSyncModal({
  profile,
  onClose,
  onDone,
}: {
  profile: ServerProfile;
  onClose: () => void;
  onDone: () => Promise<void>;
}) {
  const { pushToast, t } = useApp();
  const [files, setFiles] = useState<ServerSyncFile[]>([]);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<'selected' | 'full' | null>(null);

  useEffect(() => {
    let alive = true;
    api.previewServerSync(profile.id)
      .then((rows) => {
        if (!alive) return;
        setFiles(rows);
        setSelected(new Set(rows.filter((file) => !file.clientOnly).map((file) => file.path)));
      })
      .catch((error: Error) => {
        if (alive) pushToast('error', t('servers.syncFailed'), error.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => { alive = false; };
  }, [profile.id, pushToast, t]);

  const groups = useMemo(() => {
    const map = new Map<ServerSyncDir, ServerSyncFile[]>();
    for (const file of files) {
      const list = map.get(file.dir) ?? [];
      list.push(file);
      map.set(file.dir, list);
    }
    return ALL_DIRS
      .map((dir) => ({ dir, files: map.get(dir) ?? [] }))
      .filter((group) => group.files.length > 0);
  }, [files]);

  const toggle = (file: ServerSyncFile) => {
    if (file.clientOnly) return;
    setSelected((current) => {
      const next = new Set(current);
      if (next.has(file.path)) next.delete(file.path);
      else next.add(file.path);
      return next;
    });
  };

  const allSelectable = files.filter((file) => !file.clientOnly);
  const allSelected = allSelectable.length > 0 && allSelectable.every((file) => selected.has(file.path));

  const selectAll = () => {
    setSelected(allSelected ? new Set() : new Set(allSelectable.map((file) => file.path)));
  };

  const run = async (mode: 'selected' | 'full') => {
    if (mode === 'full' && profile.deleteExtras) {
      if (!window.confirm(t('servers.fullSyncConfirm'))) return;
    }
    setBusy(mode);
    try {
      const result = mode === 'selected'
        ? await api.syncServerProfileSelected(profile.id, Array.from(selected))
        : await api.syncServerProfile(profile.id);
      pushToast('success', t('servers.syncDone'), syncSummary(result, t));
      await onDone();
    } catch (error) {
      pushToast('error', t('servers.syncFailed'), (error as Error).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      closeable={!busy}
      title={t('servers.filesTitle')}
      eyebrow={profile.name}
      description={t('servers.filesHint')}
      size="lg"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={Boolean(busy)}>{t('common.cancel')}</button>
          <button type="button" className="btn btn-ghost" onClick={() => void run('full')} disabled={Boolean(busy) || loading}>
            {busy === 'full' ? <LoaderCircle size={15} className="spin" /> : <UploadCloud size={15} />}
            {t('servers.syncAll')}
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => void run('selected')}
            disabled={Boolean(busy) || selected.size === 0}
          >
            {busy === 'selected' ? <LoaderCircle size={15} className="spin" /> : <Files size={15} />}
            {t('servers.syncSelected')} · {selected.size}
          </button>
        </>
      }
    >
      {loading ? (
        <div className="skeleton skeleton-line wide" />
      ) : (
        <>
          <div className="file-picker-toolbar">
            <button type="button" className="btn btn-ghost btn-sm" onClick={selectAll}>
              {allSelected ? t('servers.clearSelection') : t('servers.selectAll')}
            </button>
            <span className="field-hint">
              {files.filter((file) => file.clientOnly).length > 0
                ? t('servers.syncFiltered', { count: files.filter((file) => file.clientOnly).length })
                : ''}
            </span>
          </div>
          <div className="file-picker">
            {groups.length === 0 && <p className="field-hint">{t('servers.noFiles')}</p>}
            {groups.map((group) => (
              <div className="file-group" key={group.dir}>
                <div className="file-group-head">
                  <strong>{t(DIR_KEYS[group.dir])}</strong>
                  <span>{group.files.length}</span>
                </div>
                {group.files.map((file) => (
                  <label
                    className={cn('file-row', file.clientOnly && 'is-excluded', selected.has(file.path) && 'is-selected')}
                    key={file.path}
                    title={file.reason || file.path}
                  >
                    <input
                      type="checkbox"
                      checked={selected.has(file.path)}
                      disabled={file.clientOnly}
                      onChange={() => toggle(file)}
                    />
                    <span className="file-path">{file.path}</span>
                    <span className="file-size">{formatBytes(file.size)}</span>
                    {file.clientOnly && <span className="chip chip-warn">{t('servers.clientOnly')}</span>}
                  </label>
                ))}
              </div>
            ))}
          </div>
          {files.length > 0 && (
            <p className="field-hint">{t('servers.filesCount', { count: files.length, selected: selected.size })}</p>
          )}
        </>
      )}
    </Modal>
  );
}

function ServerRollbackModal({
  profile,
  online,
  onClose,
  onDone,
}: {
  profile: ServerProfile;
  online: boolean;
  onClose: () => void;
  onDone: () => Promise<void>;
}) {
  const { pushToast, t, locale } = useApp();
  const [backups, setBackups] = useState<ServerBackup[]>([]);
  const [loading, setLoading] = useState(true);
  const [busyId, setBusyId] = useState<string | null>(null);

  useEffect(() => {
    let alive = true;
    api.getServerBackups(profile.id)
      .then((rows) => { if (alive) setBackups(rows); })
      .catch((error: Error) => {
        if (alive) pushToast('error', t('servers.rollbackFailed'), error.message);
      })
      .finally(() => { if (alive) setLoading(false); });
    return () => { alive = false; };
  }, [profile.id, pushToast, t]);

  const formatTime = (value: string) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleString(locale);
  };

  const restore = async (backup: ServerBackup) => {
    if (!window.confirm(t('servers.rollbackConfirm', { time: formatTime(backup.createdAt) }))) return;
    setBusyId(backup.id);
    try {
      await api.rollbackServer(profile.id, backup.id);
      pushToast('success', t('servers.rollbackDone'));
      await onDone();
    } catch (error) {
      pushToast('error', t('servers.rollbackFailed'), (error as Error).message);
    } finally {
      setBusyId(null);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      closeable={!busyId}
      title={t('servers.rollbackTitle')}
      eyebrow={profile.name}
      description={t('servers.rollbackBody')}
      size="lg"
      footer={<button type="button" className="btn btn-ghost" onClick={onClose} disabled={Boolean(busyId)}>{t('common.close')}</button>}
    >
      <div className="inline-banner banner-warning">
        <RotateCcw size={16} />
        <div>
          <strong>{online ? t('servers.statusRunningBlock') : t('servers.rollbackWarning')}</strong>
        </div>
      </div>
      {loading ? (
        <div className="skeleton skeleton-line wide" />
      ) : backups.length === 0 ? (
        <EmptyState compact icon={<History size={22} />} title={t('servers.rollback')} body={t('servers.noBackups')} />
      ) : (
        <div className="backup-list">
          {backups.map((backup) => (
            <div className="backup-row" key={backup.id}>
              <div className="backup-copy">
                <strong>{formatTime(backup.createdAt)}</strong>
                <span>{backup.reason || 'sync'}</span>
                <div className="chip-row">
                  {backup.dirs.map((dir) => (
                    <span className="chip" key={dir}>{DIR_KEYS[dir] ? t(DIR_KEYS[dir]) : dir}</span>
                  ))}
                </div>
              </div>
              <button
                type="button"
                className="btn btn-danger-ghost btn-sm"
                onClick={() => void restore(backup)}
                disabled={online || Boolean(busyId)}
              >
                {busyId === backup.id ? <LoaderCircle size={14} className="spin" /> : <RotateCcw size={14} />}
                {busyId === backup.id ? t('servers.restoring') : t('servers.restore')}
              </button>
            </div>
          ))}
        </div>
      )}
    </Modal>
  );
}

function syncSummary(result: ServerSyncResult, t: (key: MessageKey, vars?: Record<string, string | number>) => string) {
  const summary = t('servers.syncSummary', {
    uploaded: result.uploaded,
    skipped: result.skipped,
    deleted: result.deleted,
  });
  return result.filtered > 0
    ? `${summary} · ${t('servers.syncFiltered', { count: result.filtered })}`
    : summary;
}

function ServerForm({
  profile,
  instances,
  onClose,
  onSaved,
}: {
  profile: ServerProfile | null;
  instances: InstanceView[];
  onClose: () => void;
  onSaved: () => Promise<void>;
}) {
  const { pushToast, t } = useApp();
  const [form, setForm] = useState<ServerProfileRequest>(() => ({
    id: profile?.id,
    name: profile?.name ?? '',
    panelUrl: profile?.panelUrl ?? '',
    daemonId: profile?.daemonId ?? '',
    instanceUuid: profile?.instanceUuid ?? '',
    localInstanceId: profile?.localInstanceId ?? instances[0]?.id ?? '',
    remoteRoot: profile?.remoteRoot ?? '/',
    syncDirs: profile?.syncDirs ?? ['mods', 'config', 'defaultconfigs', 'resourcepacks'],
    deleteExtras: profile?.deleteExtras ?? false,
    apiKey: '',
  }));
  const [remote, setRemote] = useState<RemoteMcsmInstance[]>([]);
  const [testing, setTesting] = useState(false);
  const [saving, setSaving] = useState(false);

  const patch = <K extends keyof ServerProfileRequest>(key: K, value: ServerProfileRequest[K]) => {
    setForm((current) => ({ ...current, [key]: value }));
  };

  const toggleDir = (dir: ServerSyncDir) => {
    setForm((current) => ({
      ...current,
      syncDirs: current.syncDirs.includes(dir)
        ? current.syncDirs.filter((item) => item !== dir)
        : [...current.syncDirs, dir],
    }));
  };

  const selectedRemote = useMemo(() => (
    remote.find((item) => item.daemonId === form.daemonId && item.instanceUuid === form.instanceUuid)
  ), [remote, form.daemonId, form.instanceUuid]);

  const testConnection = async () => {
    setTesting(true);
    try {
      const rows = await api.listMcsmInstances({
        profileId: form.id,
        panelUrl: form.panelUrl,
        apiKey: form.apiKey,
      });
      setRemote(rows);
      if (rows.length === 1) {
        patch('daemonId', rows[0].daemonId);
        patch('instanceUuid', rows[0].instanceUuid);
        if (!form.name.trim()) patch('name', rows[0].nickname || 'MCSM');
      }
      pushToast('success', t('servers.testSuccess'));
    } catch (error) {
      pushToast('error', t('servers.testFailed'), (error as Error).message);
    } finally {
      setTesting(false);
    }
  };

  const save = async () => {
    setSaving(true);
    try {
      await api.saveServerProfile(form);
      await onSaved();
    } catch (error) {
      pushToast('error', t('servers.saveFailed'), (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={profile ? t('servers.edit') : t('servers.add')}
      eyebrow="MCSM"
      description={t('servers.description')}
      size="lg"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={saving}>{t('common.cancel')}</button>
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving || !form.localInstanceId}>
            {saving && <LoaderCircle size={15} className="spin" />}
            {t('common.save')}
          </button>
        </>
      }
    >
      <div className="settings-fields">
        <label className="field">
          <span className="field-label">{t('servers.name')}</span>
          <input
            className="text-input"
            value={form.name}
            onChange={(event) => patch('name', event.target.value)}
            placeholder={t('servers.namePlaceholder')}
          />
        </label>

        <label className="field">
          <span className="field-label">{t('servers.panelUrl')}</span>
          <input
            className="text-input mono"
            value={form.panelUrl}
            onChange={(event) => patch('panelUrl', event.target.value)}
            placeholder="http://192.168.1.10:23333"
            autoComplete="off"
          />
          <span className="field-hint">{t('servers.panelUrlHint')}</span>
        </label>

        <label className="field">
          <span className="field-label"><KeyRound size={14} /> {t('servers.apiKey')}</span>
          <input
            className="text-input mono"
            type="password"
            value={form.apiKey ?? ''}
            onChange={(event) => patch('apiKey', event.target.value)}
            placeholder={profile?.apiKeySet ? t('servers.apiKeyKeep') : ''}
            autoComplete="new-password"
          />
          <span className="field-hint">{profile?.apiKeySet ? t('servers.apiKeyKeep') : t('servers.apiKeyHint')}</span>
        </label>

        <div className="field">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void testConnection()} disabled={testing || !form.panelUrl}>
            {testing ? <LoaderCircle size={14} className="spin" /> : <RefreshCw size={14} />}
            {testing ? t('servers.testing') : t('servers.test')}
          </button>
          <span className="field-hint">{t('servers.apiKeyHint')}</span>
        </div>

        <label className="field">
          <span className="field-label">{t('servers.pickInstance')}</span>
          <select
            className="text-input"
            value={selectedRemote ? `${form.daemonId}::${form.instanceUuid}` : ''}
            onChange={(event) => {
              const [daemonId, instanceUuid] = event.target.value.split('::');
              patch('daemonId', daemonId);
              patch('instanceUuid', instanceUuid);
              const match = remote.find((item) => item.daemonId === daemonId && item.instanceUuid === instanceUuid);
              if (match && !form.name.trim()) patch('name', match.nickname || 'MCSM');
            }}
          >
            <option value="">{t('servers.manualInstance')}</option>
            {remote.map((item) => (
              <option key={`${item.daemonId}::${item.instanceUuid}`} value={`${item.daemonId}::${item.instanceUuid}`}>
                {item.nickname || item.instanceUuid} · {item.daemonId}
              </option>
            ))}
          </select>
        </label>

        <div className="settings-grid-2">
          <label className="field">
            <span className="field-label">{t('servers.daemonId')}</span>
            <input className="text-input mono" value={form.daemonId} onChange={(event) => patch('daemonId', event.target.value)} />
          </label>
          <label className="field">
            <span className="field-label">{t('servers.instanceId')}</span>
            <input className="text-input mono" value={form.instanceUuid} onChange={(event) => patch('instanceUuid', event.target.value)} />
          </label>
        </div>

        <label className="field">
          <span className="field-label">{t('servers.localInstance')}</span>
          <select className="text-input" value={form.localInstanceId} onChange={(event) => patch('localInstanceId', event.target.value)}>
            {instances.map((instance) => (
              <option key={instance.id} value={instance.id}>
                {instance.name} · {instance.mcVersion} · {instance.loader}
              </option>
            ))}
          </select>
        </label>

        <label className="field">
          <span className="field-label">{t('servers.remoteRoot')}</span>
          <input className="text-input mono" value={form.remoteRoot} onChange={(event) => patch('remoteRoot', event.target.value)} />
          <span className="field-hint">{t('servers.remoteRootHint')}</span>
        </label>

        <div className="field">
          <span className="field-label">{t('servers.syncDirs')}</span>
          <div className="chip-row">
            {ALL_DIRS.map((dir) => (
              <button
                key={dir}
                type="button"
                className={cn('chip', 'chip-button', form.syncDirs.includes(dir) && 'is-active')}
                onClick={() => toggleDir(dir)}
              >
                {t(DIR_KEYS[dir])}
              </button>
            ))}
          </div>
        </div>

        <label className="toggle-row">
          <span>
            <strong>{t('servers.deleteExtras')}</strong>
            <small>{t('servers.deleteExtrasHint')}</small>
          </span>
          <input
            type="checkbox"
            className="switch"
            checked={form.deleteExtras}
            onChange={(event) => patch('deleteExtras', event.target.checked)}
          />
        </label>
      </div>
    </Modal>
  );
}
