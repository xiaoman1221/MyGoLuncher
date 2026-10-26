import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Download,
  LoaderCircle,
  MessageCircle,
  Play,
  RadioTower,
  RefreshCw,
  ScanLine,
  Server,
  Share2,
  UserRound,
  Users,
  Wifi,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { LanSyncModal } from '../components/LanSyncModal';
import { LanVersionInstallModal } from '../components/LanVersionInstallModal';
import { LanToolsModal } from '../components/LanToolsModal';
import { cn } from '../lib/format';
import type { MessageKey } from '../lib/i18n';
import type {
  InstanceView,
  LanPeer,
  LanShareDir,
  LanSnapshot,
  LanWorld,
} from '../lib/types';

const SHARE_DIRS: LanShareDir[] = ['mods', 'shaderpacks', 'resourcepacks', 'config'];

const SHARE_DIR_KEYS: Record<LanShareDir, MessageKey> = {
  mods: 'servers.dir.mods',
  shaderpacks: 'servers.dir.shaderpacks',
  resourcepacks: 'servers.dir.resourcepacks',
  config: 'servers.dir.config',
};

function isShareDir(value: string): value is LanShareDir {
  return value === 'mods' || value === 'shaderpacks' || value === 'resourcepacks' || value === 'config';
}

function normalizeVersion(value: string): string {
  return value.toLowerCase().replace(/\([^)]*\)|\[[^\]]*\]/g, '').replace(/\s+/g, '').trim();
}

function versionMatches(local: string, remote: string): boolean {
  const normalizedRemote = normalizeVersion(remote);
  if (!normalizedRemote) return true;
  return normalizeVersion(local) === normalizedRemote;
}

const emptySnapshot: LanSnapshot = {
  running: false,
  broadcast: true,
  sharing: false,
  worlds: [],
  peers: [],
};

export function LanPage() {
  const {
    instances, selectedId, selectInstance, launchToServer, pushToast, t, locale,
  } = useApp();
  const [snapshot, setSnapshot] = useState<LanSnapshot>(emptySnapshot);
  const [starting, setStarting] = useState(true);
  const [scanning, setScanning] = useState(false);
  const [joining, setJoining] = useState<string | null>(null);
  const [manual, setManual] = useState('');
  const [joinInstance, setJoinInstance] = useState(selectedId ?? instances[0]?.id ?? '');
  const [shareInstance, setShareInstance] = useState(selectedId ?? instances[0]?.id ?? '');
  const [shareDirs, setShareDirs] = useState<LanShareDir[]>(['mods', 'shaderpacks', 'resourcepacks']);
  const [shareFull, setShareFull] = useState(false);
  const [shareBusy, setShareBusy] = useState(false);
  const [syncPeer, setSyncPeer] = useState<LanPeer | null>(null);
  const [installPeer, setInstallPeer] = useState<LanPeer | null>(null);
  const [toolsOpen, setToolsOpen] = useState(false);
  const startedRef = useRef(false);
  const sharingRef = useRef(false);
  sharingRef.current = snapshot.sharing;

  useEffect(() => {
    if (selectedId && instances.some((instance) => instance.id === selectedId)) {
      setJoinInstance(selectedId);
    }
  }, [selectedId, instances]);

  useEffect(() => {
    let active = true;
    const unsubscribe = onEvent<LanSnapshot>('lan:changed', (next) => {
      if (active) setSnapshot(next);
    });
    api.getLanSnapshot()
      .then((current) => { if (active) setSnapshot(current); })
      .finally(() => { if (active) setStarting(false); });
    return () => {
      active = false;
      unsubscribe();
    };
  }, []);

  useEffect(() => {
    if (!joinInstance) {
      setStarting(false);
      return;
    }
    let active = true;
    // Discovery runs in the background, but advertising is page-scoped:
    // enable it for the selected instance and turn it off on unmount.
    setStarting(true);
    const broadcast = startedRef.current ? snapshot.broadcast : true;
    startedRef.current = true;
    api.startLanDiscovery(joinInstance, broadcast)
      .then((current) => { if (active) setSnapshot(current); })
      .catch((error) => {
        if (active) pushToast('error', t('lan.startFailed'), (error as Error).message);
      })
      .finally(() => { if (active) setStarting(false); });
    return () => { active = false; };
  }, [joinInstance]);

  useEffect(() => () => {
    // Keep advertising while this launcher is still sharing files; otherwise
    // leaving the page would make the share disappear from other clients.
    if (!sharingRef.current) void api.setLanBroadcast(false).catch(() => {});
  }, []);

  useEffect(() => {
    if (instances.length === 0) {
      setShareInstance('');
      return;
    }
    if (!instances.some((instance) => instance.id === shareInstance)) {
      const next = selectedId && instances.some((instance) => instance.id === selectedId)
        ? selectedId
        : instances[0].id;
      setShareInstance(next);
    }
  }, [instances, selectedId, shareInstance]);

  const shareKey = (snapshot.shareDirs ?? []).join(',');
  useEffect(() => {
    if (!snapshot.sharing) return;
    if (snapshot.sharedInstanceId && instances.some((instance) => instance.id === snapshot.sharedInstanceId)) {
      setShareInstance((current) => current === snapshot.sharedInstanceId ? current : snapshot.sharedInstanceId!);
    }
    const all = (snapshot.shareDirs ?? []).includes('*');
    setShareFull(all);
    if (all) return;
    const dirs = (snapshot.shareDirs ?? []).filter(isShareDir);
    if (dirs.length > 0) {
      setShareDirs((current) => current.join(',') === dirs.join(',') ? current : dirs);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [snapshot.sharing, snapshot.sharedInstanceId, shareKey, instances]);

  const toggleBroadcast = async (enabled: boolean) => {
    try {
      const next = await api.setLanBroadcast(enabled);
      setSnapshot(next);
    } catch (error) {
      pushToast('error', t('lan.startFailed'), (error as Error).message);
    }
  };

  const scan = async () => {
    setScanning(true);
    try {
      setSnapshot(await api.scanLanSubnets());
    } catch (error) {
      pushToast('error', t('lan.scanFailed'), (error as Error).message);
    } finally {
      setScanning(false);
    }
  };

  const join = async (address: string, key: string, worldVersion?: string) => {
    const target = address.trim();
    if (!target) return;
    if (!joinInstance) {
      pushToast('error', t('lan.joinFailed'), t('lan.noInstance'));
      return;
    }
    const instance = instances.find((item) => item.id === joinInstance);
    if (worldVersion && instance && !versionMatches(instance.mcVersion, worldVersion)) {
      pushToast('error', t('lan.versionMismatch'), t('lan.versionMismatchBody', {
        world: worldVersion,
        local: instance.mcVersion,
      }));
      return;
    }
    setJoining(key);
    try {
      if (joinInstance !== selectedId) selectInstance(joinInstance);
      await launchToServer(joinInstance, target);
    } catch (error) {
      pushToast('error', t('lan.joinFailed'), (error as Error).message);
    } finally {
      setJoining(null);
    }
  };

  const toggleShare = async (enabled: boolean) => {
    if (enabled && !shareInstance) {
      pushToast('error', t('lan.shareFailed'), t('lan.noInstance'));
      return;
    }
    if (enabled && !shareFull && shareDirs.length === 0) {
      pushToast('error', t('lan.shareFailed'), t('lan.shareDirsRequired'));
      return;
    }
    setShareBusy(true);
    try {
      const next = enabled
        ? await api.startLanShare(shareInstance, shareFull ? ['*'] : shareDirs)
        : await api.stopLanShare();
      setSnapshot(next);
      pushToast(enabled ? 'success' : 'info', enabled ? t('lan.shareStarted') : t('lan.shareStopped'));
    } catch (error) {
      pushToast('error', t('lan.shareFailed'), (error as Error).message);
    } finally {
      setShareBusy(false);
    }
  };

  const toggleShareDir = (dir: LanShareDir) => {
    if (snapshot.sharing || shareFull) return;
    setShareDirs((current) => current.includes(dir)
      ? current.filter((item) => item !== dir)
      : [...current, dir]);
  };

  const formatClock = (value: string) => {
    const date = new Date(value);
    return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString(locale);
  };

  const peerCount = snapshot.peers.length;
  const playerNames = useMemo(() => {
    const names = new Set<string>();
    snapshot.worlds.forEach((world) => world.players.forEach((player) => names.add(player.name)));
    return names.size;
  }, [snapshot.worlds]);

  const sharedInstanceName = instances.find((instance) => instance.id === snapshot.sharedInstanceId)?.name
    ?? shareInstance
    ?? '';

  return (
    <div className="page lan-page">
      <PageHeader
        kicker="LAN"
        title={<>{t('lan.title')} <span className="title-accent">/</span> LAN</>}
        description={t('lan.description')}
        actions={
          <div className="header-actions">
            <button type="button" className="btn btn-ghost" onClick={() => setToolsOpen(true)} disabled={starting}>
              <MessageCircle size={15} /> {t('lan.toolsTitle')}
            </button>
            <label className="lan-broadcast-toggle">
              <span>{t('lan.broadcast')}</span>
              <input
                type="checkbox"
                className="switch"
                checked={snapshot.broadcast}
                onChange={(event) => void toggleBroadcast(event.target.checked)}
              />
            </label>
            <button type="button" className="btn btn-ghost" onClick={() => void scan()} disabled={scanning || starting}>
              {scanning ? <LoaderCircle size={15} className="spin" /> : <ScanLine size={15} />}
              {scanning ? t('lan.scanning') : t('lan.scan')}
            </button>
          </div>
        }
      />

      {snapshot.warnings?.map((warning) => (
        <div className="inline-banner banner-warning" key={warning}>
          <Wifi size={17} />
          <div><span>{warning}</span></div>
        </div>
      ))}

      <section className="panel lan-join-panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2>{t('lan.manual')}</h2>
            <p>{t('lan.manualHint')}</p>
          </div>
          <span className={cn('status-pill', snapshot.running ? 'is-online' : 'is-idle')}>
            {snapshot.running ? t('lan.running') : t('servers.status.unknown')}
          </span>
        </div>
        <div className="lan-join-row">
          <label className="field lan-instance-field">
            <span className="field-label">{t('lan.selectedInstance')}</span>
            <select
              className="text-input"
              value={joinInstance}
              onChange={(event) => {
                setJoinInstance(event.target.value);
                if (event.target.value) selectInstance(event.target.value);
              }}
            >
              {instances.length === 0 && <option value="">{t('lan.noInstance')}</option>}
              {instances.map((instance) => (
                <option key={instance.id} value={instance.id}>
                  {instance.name} · {instance.mcVersion} · {instance.loader}
                </option>
              ))}
            </select>
          </label>
          <label className="field lan-address-field">
            <span className="field-label">{t('lan.addressPlaceholder')}</span>
            <input
              className="text-input mono"
              value={manual}
              onChange={(event) => setManual(event.target.value)}
              placeholder="192.168.1.20:25565"
              onKeyDown={(event) => {
                if (event.nativeEvent.isComposing || event.keyCode === 229) return;
                if (event.key === 'Enter') void join(manual, 'manual');
              }}
            />
          </label>
          <button
            type="button"
            className="btn btn-primary lan-join-button"
            onClick={() => void join(manual, 'manual')}
            disabled={joining === 'manual' || !manual.trim() || !joinInstance}
          >
            {joining === 'manual' ? <LoaderCircle size={15} className="spin" /> : <Play size={15} />}
            {t('lan.join')}
          </button>
        </div>
        <p className="field-hint">{t('lan.hint')} {t('lan.firewallHint')}</p>
      </section>

      <section className="panel lan-share-panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2><Share2 size={17} /> {t('lan.shareTitle')}</h2>
            <p>{t('lan.shareBody')}</p>
          </div>
          <label className="lan-broadcast-toggle">
            <span>{snapshot.sharing ? t('lan.shareOn') : t('lan.shareOff')}</span>
            <input
              type="checkbox"
              className="switch"
              checked={snapshot.sharing}
              disabled={shareBusy || !shareInstance || instances.length === 0}
              onChange={(event) => void toggleShare(event.target.checked)}
            />
          </label>
        </div>
        <div className="lan-share-row">
          <label className="field lan-share-instance">
            <span className="field-label">{t('lan.shareInstance')}</span>
            <select
              className="text-input"
              value={shareInstance}
              disabled={snapshot.sharing || shareBusy}
              onChange={(event) => setShareInstance(event.target.value)}
            >
              {instances.length === 0 && <option value="">{t('lan.noInstance')}</option>}
              {instances.map((instance) => (
                <option key={instance.id} value={instance.id}>
                  {instance.name} · {instance.mcVersion} · {instance.loader}
                </option>
              ))}
            </select>
          </label>
          <div className="field">
            <span className="field-label">{t('lan.shareDirs')}</span>
            <div className="chip-row">
              {SHARE_DIRS.map((dir) => (
                <button
                  key={dir}
                  type="button"
                  className={cn('chip', 'chip-button', shareDirs.includes(dir) && 'is-active')}
                  onClick={() => toggleShareDir(dir)}
                  disabled={snapshot.sharing || shareBusy || shareFull}
                >
                  {t(SHARE_DIR_KEYS[dir])}
                </button>
              ))}
            </div>
          </div>
        </div>
        <label className="toggle-row lan-full-share">
          <span>
            <strong>{t('lan.shareFull')}</strong>
            <small>{t('lan.shareFullHint')}</small>
          </span>
          <input
            type="checkbox"
            className="switch"
            checked={shareFull}
            disabled={snapshot.sharing || shareBusy}
            onChange={(event) => setShareFull(event.target.checked)}
          />
        </label>
        <p className="field-hint">{t('lan.shareHint')}</p>
        {snapshot.sharing && (
          <p className="field-hint">{t('lan.shareActive', { instance: sharedInstanceName })}</p>
        )}
        {snapshot.sharing && !snapshot.broadcast && (
          <p className="field-hint lan-warning-hint">{t('lan.shareNeedsBroadcast')}</p>
        )}
      </section>

      <section className="lan-stats">
        <div className="lan-stat">
          <RadioTower size={18} />
          <span>{t('lan.worlds')}</span>
          <strong>{snapshot.worlds.length}</strong>
        </div>
        <div className="lan-stat">
          <Users size={18} />
          <span>{t('lan.players')}</span>
          <strong>{playerNames}</strong>
        </div>
        <div className="lan-stat">
          <UserRound size={18} />
          <span>{t('lan.launcherPlayers')}</span>
          <strong>{peerCount}</strong>
        </div>
      </section>

      <section>
        <div className="section-head">
          <div>
            <h2>{t('lan.worlds')}</h2>
            <p>{t('lan.noWorldsBody')}</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void scan()} disabled={scanning || starting}>
            <RefreshCw size={14} className={scanning ? 'spin' : undefined} />
            {t('lan.refresh')}
          </button>
        </div>
        {starting ? (
          <div className="skeleton skeleton-line wide" />
        ) : snapshot.worlds.length === 0 ? (
          <EmptyState
            icon={<Server size={24} />}
            title={t('lan.noWorlds')}
            body={t('lan.noWorldsBody')}
            action={(
              <button type="button" className="btn btn-ghost" onClick={() => void scan()} disabled={scanning || starting}>
                <ScanLine size={15} /> {t('lan.scan')}
              </button>
            )}
          />
        ) : (
          <div className="lan-world-grid">
            {snapshot.worlds.map((world) => (
              <WorldCard
                key={world.id}
                world={world}
                busy={joining === world.id}
                disabled={!joinInstance}
                onJoin={() => void join(world.address, world.id, world.version)}
                formatClock={formatClock}
              />
            ))}
          </div>
        )}
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('lan.launcherPlayers')}</h2>
          <p>{t('lan.broadcastHint')}</p>
        </div>
        {snapshot.peers.length === 0 ? (
          <EmptyState
            compact
            icon={<Users size={22} />}
            title={t('lan.noPeers')}
            body={t('lan.noPeersBody')}
          />
        ) : (
          <div className="lan-peer-list">
            {snapshot.peers.map((peer) => (
              <div className="lan-peer" key={peer.id}>
                <span className="lan-peer-avatar"><UserRound size={17} /></span>
                <div className="lan-peer-copy">
                  <strong>{peer.name}</strong>
                  <span>{t('lan.peerLine', { instance: peer.instance || 'Minecraft', version: peer.mcVersion || '—' })}</span>
                  {peer.canSync && peer.syncDirs && peer.syncDirs.length > 0 && (
                    <span className="lan-peer-dirs">
                      <Download size={11} /> {peer.syncDirs.map((dir) => dir === '*' ? t('lan.fullVersion') : dir).join(' · ')}
                    </span>
                  )}
                </div>
                {peer.canSync && (
                  <button
                    type="button"
                    className="btn btn-primary btn-sm lan-peer-sync"
                    onClick={() => (peer.syncDirs?.includes('*') ? setInstallPeer(peer) : setSyncPeer(peer))}
                  >
                    <Download size={14} /> {peer.syncDirs?.includes('*') ? t('lan.installVersion') : t('lan.sync')}
                  </button>
                )}
                <div className="lan-peer-meta">
                  <code>{peer.address}</code>
                  <span>{formatClock(peer.lastSeen)}</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      {syncPeer && (
        <LanSyncModal
          peer={syncPeer}
          instances={instances}
          selectedId={selectedId}
          onClose={() => setSyncPeer(null)}
        />
      )}

      {installPeer && (
        <LanVersionInstallModal
          peer={installPeer}
          onClose={() => setInstallPeer(null)}
        />
      )}

      {toolsOpen && (
        <LanToolsModal
          peers={snapshot.peers}
          onClose={() => setToolsOpen(false)}
        />
      )}
    </div>
  );
}

function WorldCard({
  world,
  busy,
  disabled,
  onJoin,
  formatClock,
}: {
  world: LanWorld;
  busy: boolean;
  disabled: boolean;
  onJoin: () => void;
  formatClock: (value: string) => string;
}) {
  const { t } = useApp();
  const players = world.players ?? [];
  return (
    <article className="panel lan-world-card">
      <div className="lan-world-head">
        <div>
          <h3>{world.motd || world.address}</h3>
          <span className="lan-address">{world.address}</span>
        </div>
        <span className={cn('status-pill', world.online > 0 ? 'is-online' : 'is-idle')}>
          {t('lan.online', { online: world.online, max: world.max })}
        </span>
      </div>
      <div className="lan-world-meta">
        <span className="chip">{world.version || 'Minecraft'}</span>
        <span className="chip">{world.source === 'scan' ? t('lan.source.scan') : t('lan.source.lan')}</span>
        {world.hostName && <span className="chip is-key">{t('lan.openedBy', { name: world.hostName })}</span>}
        <span className="lan-seen">{t('lan.lastSeen', { time: formatClock(world.lastSeen) })}</span>
      </div>
      <div className="lan-players">
        <span className="field-label">{t('lan.players')}</span>
        {players.length > 0 ? (
          <div className="chip-row">
            {players.map((player) => (
              <span className="chip" key={player.name}>{player.name}</span>
            ))}
          </div>
        ) : (
          <span className="field-hint">{t('lan.noPlayers')}</span>
        )}
      </div>
      <div className="lan-world-actions">
        <button type="button" className="btn btn-primary btn-sm" onClick={onJoin} disabled={disabled || busy}>
          {busy ? <LoaderCircle size={14} className="spin" /> : <Play size={14} />}
          {t('lan.join')}
        </button>
      </div>
    </article>
  );
}
