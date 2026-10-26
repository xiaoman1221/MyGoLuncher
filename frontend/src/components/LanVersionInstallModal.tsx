import { useEffect, useState } from 'react';
import { Download, LoaderCircle, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';
import { Modal } from './Modal';
import type { LanPeer, LanVersionPreview } from '../lib/types';

function formatBytes(value: number): string {
  if (value >= 1024 * 1024 * 1024) return (value / (1024 * 1024 * 1024)).toFixed(1) + ' GB';
  if (value >= 1024 * 1024) return (value / (1024 * 1024)).toFixed(1) + ' MB';
  if (value >= 1024) return (value / 1024).toFixed(0) + ' KB';
  return String(value) + ' B';
}

export function LanVersionInstallModal({ peer, onClose }: { peer: LanPeer; onClose: () => void }) {
  const { pushToast, refreshInstances, selectInstance, t } = useApp();
  const [preview, setPreview] = useState<LanVersionPreview | null>(null);
  const [loading, setLoading] = useState(true);
  const [installing, setInstalling] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    setLoading(true);
    setError('');
    api.previewLanVersion(peer.id)
      .then((next) => { if (active) setPreview(next); })
      .catch((reason: Error) => { if (active) setError(reason.message); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [peer.id]);

  const install = async () => {
    if (!preview) return;
    setInstalling(true);
    setError('');
    try {
      const result = await api.installLanVersion(peer.id, preview.digest);
      await refreshInstances();
      selectInstance(result.instance.id);
      pushToast('success', t('lan.installVersionDone'), result.instance.name);
      onClose();
    } catch (reason) {
      setError((reason as Error).message);
      pushToast('error', t('lan.installVersionFailed'), (reason as Error).message);
    } finally {
      setInstalling(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      closeable={!installing}
      title={t('lan.installVersionTitle')}
      eyebrow="VERSION SHARE"
      description={t('lan.installVersionBody')}
      size="md"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={installing}>{t('common.cancel')}</button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => void install()}
            disabled={installing || loading || !preview}
          >
            {installing ? <LoaderCircle size={15} className="spin" /> : <Download size={15} />}
            {installing ? t('lan.installingVersion') : t('lan.installVersion')}
          </button>
        </>
      }
    >
      <div className="settings-fields">
        <div className="lan-sync-source">
          <span className="field-label">{t('lan.syncSource')}</span>
          <strong>{preview?.peer.name || peer.instance || 'Minecraft'}</strong>
          <div className="chip-row">
            <span className="chip">{preview?.peer.mcVersion || peer.mcVersion || '—'}</span>
            <span className="chip">{loaderLabel(preview?.peer.loader ?? peer.loader ?? 'vanilla', t('loader.vanilla'))}</span>
            <span className="chip is-key">{t('lan.fullVersion')}</span>
          </div>
        </div>

        {loading ? (
          <div className="lan-sync-loading">
            <LoaderCircle size={16} className="spin" /> {t('lan.readingVersion')}
          </div>
        ) : preview ? (
          <>
            <div className="lan-diff-stats">
              <div><span>{t('lan.files')}</span><strong>{preview.files}</strong></div>
              <div className="is-wide"><span>{t('lan.totalSize')}</span><strong>{formatBytes(preview.bytes)}</strong></div>
            </div>
            {preview.warnings.map((warning) => (
              <div className="inline-banner banner-warning" key={warning}>
                <TriangleAlert size={15} />
                <div><span>{warning}</span></div>
              </div>
            ))}
            <p className="field-hint">{t('lan.installVersionHint')}</p>
          </>
        ) : (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div><span>{error || t('lan.installVersionFailed')}</span></div>
          </div>
        )}
        {error && preview && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div><span>{error}</span></div>
          </div>
        )}
      </div>
    </Modal>
  );
}
