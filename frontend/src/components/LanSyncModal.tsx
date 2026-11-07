import { useEffect, useMemo, useState } from 'react';
import { Download, LoaderCircle, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { cn, loaderLabel } from '../lib/format';
import { Modal } from './Modal';
import type { InstanceView, LanPeer, LanSyncPreview } from '../lib/types';

interface LanSyncModalProps {
  peer: LanPeer;
  instances: InstanceView[];
  selectedId: string | null;
  onClose: () => void;
}

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return (value / (1024 * 1024)).toFixed(1) + ' MB';
  if (value >= 1024) return (value / 1024).toFixed(0) + ' KB';
  return String(value) + ' B';
}

function normalizeLoader(value?: string) {
  return (value ?? '').trim().toLowerCase() || 'vanilla';
}

export function LanSyncModal({ peer, instances, selectedId, onClose }: LanSyncModalProps) {
  const { pushToast, refreshInstances, t } = useApp();
  const [syncing, setSyncing] = useState(false);
  const [loadingPreview, setLoadingPreview] = useState(false);
  const [preview, setPreview] = useState<LanSyncPreview | null>(null);
  const [previewError, setPreviewError] = useState('');

  const compatible = useMemo(() => instances.filter((instance) => {
    if (peer.mcVersion && instance.mcVersion !== peer.mcVersion) return false;
    return normalizeLoader(instance.loader) === normalizeLoader(peer.loader);
  }), [instances, peer]);

  const orderedInstances = useMemo(() => {
    const compatibleIds = new Set(compatible.map((instance) => instance.id));
    return [...instances].sort((left, right) => (
      Number(compatibleIds.has(right.id)) - Number(compatibleIds.has(left.id))
    ));
  }, [instances, compatible]);

  const [targetId, setTargetId] = useState(() => {
    if (selectedId && compatible.some((instance) => instance.id === selectedId)) return selectedId;
    return compatible[0]?.id ?? orderedInstances[0]?.id ?? '';
  });

  useEffect(() => {
    if (!instances.some((instance) => instance.id === targetId)) {
      setTargetId(compatible[0]?.id ?? instances[0]?.id ?? '');
    }
  }, [instances, compatible, targetId]);

  useEffect(() => {
    if (!targetId) {
      setPreview(null);
      return;
    }
    let alive = true;
    setLoadingPreview(true);
    setPreviewError('');
    api.previewLanSync(peer.id, targetId)
      .then((result) => {
        if (alive) setPreview(result);
      })
      .catch((error: Error) => {
        if (alive) {
          setPreview(null);
          setPreviewError(error.message);
        }
      })
      .finally(() => {
        if (alive) setLoadingPreview(false);
      });
    return () => { alive = false; };
  }, [peer.id, targetId, pushToast, t]);

  const changeCount = preview ? preview.plan.newFiles + preview.plan.changedFiles : 0;
  const downloadBytes = preview ? preview.plan.newBytes + preview.plan.changedBytes : 0;

  const start = async () => {
    if (!targetId || !preview) return;
    setSyncing(true);
    try {
      const result = await api.syncFromLanPeer(peer.id, targetId, preview.digest);
      pushToast('success', t('lan.syncDone'), t('lan.syncSummary', {
        downloaded: result.downloaded,
        skipped: result.skipped,
      }));
      await refreshInstances();
      onClose();
    } catch (error) {
      pushToast('error', t('lan.syncFailed'), (error as Error).message);
      if ((error as Error).message.includes('重新确认')) {
        api.previewLanSync(peer.id, targetId).then(setPreview).catch(() => {});
      }
    } finally {
      setSyncing(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      closeable={!syncing}
      title={t('lan.syncTitle', { name: peer.name })}
      eyebrow="LAN SYNC"
      description={t('lan.syncBody')}
      size="lg"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={syncing}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => void start()}
            disabled={syncing || loadingPreview || !preview || preview.blocking || preview.running || (changeCount === 0 && !preview.loaderVersionMismatch && !preview.runtimeMissing)}
          >
            {syncing ? <LoaderCircle size={15} className="spin" /> : <Download size={15} />}
            {syncing
              ? t('lan.syncing')
              : preview && changeCount === 0 && !preview.loaderVersionMismatch
                ? t('lan.syncNoChanges')
                : t('lan.syncConfirm')}
          </button>
        </>
      }
    >
      <div className="settings-fields">
        <div className="lan-sync-source">
          <span className="field-label">{t('lan.syncSource')}</span>
          <strong>{peer.instance || 'Minecraft'}</strong>
          <div className="chip-row">
            <span className="chip">{peer.mcVersion || '—'}</span>
            <span className="chip">{loaderLabel(peer.loader ?? 'vanilla', t('loader.vanilla'))}</span>
            {(peer.syncDirs ?? []).map((dir) => <span key={dir} className="chip">{dir}</span>)}
          </div>
        </div>

        {orderedInstances.length > 0 ? (
          <label className="field">
            <span className="field-label">{t('lan.syncTarget')}</span>
            <select className="text-input" value={targetId} onChange={(event) => setTargetId(event.target.value)}>
              {orderedInstances.map((instance) => {
                const match = compatible.some((item) => item.id === instance.id);
                return (
                  <option key={instance.id} value={instance.id}>
                    {instance.name} · {instance.mcVersion} · {loaderLabel(instance.loader, t('loader.vanilla'))}
                    {match ? '' : ' · ' + t('lan.syncIncompatible')}
                  </option>
                );
              })}
            </select>
            <span className="field-hint">{t('lan.syncConfirmHint')}</span>
          </label>
        ) : (
          <div className="inline-banner banner-warning">
            <div>
              <strong>{t('lan.syncNoMatch')}</strong>
              <span>{t('lan.syncNoMatchBody', {
                version: peer.mcVersion || '—',
                loader: peer.loader || 'Vanilla',
              })}</span>
            </div>
          </div>
        )}

        {previewError && !loadingPreview && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div>
              <strong>{t('lan.syncPreviewFailed')}</strong>
              <span>{previewError}</span>
            </div>
          </div>
        )}

        {loadingPreview ? (
          <div className="lan-sync-loading">
            <LoaderCircle size={16} className="spin" /> {t('lan.syncChecking')}
          </div>
        ) : preview ? (
          <>
            <div className="lan-sync-versions">
              <div>
                <span>{t('lan.syncPeerVersion')}</span>
                <strong>{preview.peer.mcVersion || '—'}</strong>
                <small>{loaderLabel(preview.peer.loader, t('loader.vanilla'))} {preview.peer.loaderVersion}</small>
              </div>
              <span className="lan-sync-arrow">→</span>
              <div>
                <span>{t('lan.syncLocalVersion')}</span>
                <strong>{preview.target.mcVersion || '—'}</strong>
                <small>{loaderLabel(preview.target.loader, t('loader.vanilla'))} {preview.target.loaderVersion}</small>
              </div>
            </div>

            {(preview.blocking || preview.versionMismatch || preview.loaderMismatch) && (
              <div className="inline-banner banner-error">
                <TriangleAlert size={16} />
                <div>
                  <strong>{t('lan.syncBlocking')}</strong>
                  <span>{preview.blockingReason || t('lan.syncNoMatchBody', {
                    version: preview.peer.mcVersion || '—',
                    loader: loaderLabel(preview.peer.loader, t('loader.vanilla')),
                  })}</span>
                </div>
              </div>
            )}

            <div className="lan-diff-stats">
              <div><span>{t('lan.syncNewFiles', { count: preview.plan.newFiles })}</span><strong>+{preview.plan.newFiles}</strong></div>
              <div><span>{t('lan.syncChangedFiles', { count: preview.plan.changedFiles })}</span><strong>~{preview.plan.changedFiles}</strong></div>
              <div><span>{t('lan.syncSameFiles', { count: preview.plan.sameFiles })}</span><strong>{preview.plan.sameFiles}</strong></div>
              <div><span>{t('lan.syncExtraFiles', { count: preview.plan.extraFiles })}</span><strong>{preview.plan.extraFiles}</strong></div>
              <div className="is-wide">
                <span>{t('lan.syncBytes', { size: formatBytes(downloadBytes) })}</span>
                <strong>{t('lan.syncTotalFiles', { count: preview.plan.totalFiles })}</strong>
              </div>
            </div>

            {preview.plan.changes.length > 0 && (
              <div className="lan-diff-list">
                <span className="field-label">{t('lan.syncChangeList')}</span>
                {preview.plan.changes.slice(0, 8).map((change) => (
                  <div className="lan-diff-row" key={change.path}>
                    <span className={cn('chip', change.state === 'new' ? 'is-key' : 'chip-warn')}>
                      {change.state === 'new' ? t('lan.syncFileNew') : t('lan.syncFileChanged')}
                    </span>
                    <code title={change.path}>{change.path}</code>
                    <span>{formatBytes(change.remoteSize)}</span>
                  </div>
                ))}
                {preview.plan.changes.length > 8 && (
                  <span className="field-hint">{t('lan.syncAndMore', { count: preview.plan.changes.length - 8 })}</span>
                )}
              </div>
            )}

            {preview.warnings.map((warning) => (
              <div className="inline-banner banner-warning" key={warning}>
                <TriangleAlert size={15} />
                <div><span>{warning}</span></div>
              </div>
            ))}
          </>
        ) : (
          <p className="field-hint">{t('lan.syncPreviewHint')}</p>
        )}
      </div>
    </Modal>
  );
}
