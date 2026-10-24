import { useCallback, useEffect, useState } from 'react';
import { LoaderCircle, RefreshCw, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { Modal } from './Modal';
import type { InstanceView, ModUpdateFile, ModUpdateReport } from '../lib/types';

interface ModUpdatesModalProps {
  instance: InstanceView;
  onClose: () => void;
  onUpdated: () => Promise<void>;
}

export function ModUpdatesModal({ instance, onClose, onUpdated }: ModUpdatesModalProps) {
  const { pushToast, t } = useApp();
  const [report, setReport] = useState<ModUpdateReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [updatingAll, setUpdatingAll] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setReport(await api.checkModUpdates(instance.id));
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setLoading(false);
    }
  }, [instance.id]);

  useEffect(() => {
    void load();
  }, [load]);

  const applyOne = async (file: ModUpdateFile, force = false): Promise<boolean> => {
    setBusy(file.relativePath);
    try {
      const result = await api.applyModUpdate({
        instanceId: instance.id, relativePath: file.relativePath,
        source: file.source ?? '', projectId: file.projectId ?? '', versionId: file.versionId ?? '', force,
      });
      const rollbackFile = result.rollbackPath?.split(/[\\/]/).pop() ?? '';
      const rollbackNote = rollbackFile
        ? ' · ' + t('modUpdates.rollbackSaved', { file: rollbackFile })
        : '';
      pushToast('success', t('modUpdates.done'), t('modUpdates.doneDetail', {
        name: result.versionName || file.name,
        version: file.latestVersion || '',
      }) + rollbackNote);
      await onUpdated();
      return true;
    } catch (reason) {
      const message = (reason as Error).message;
      if (!force && message.includes('检测到模组冲突') && window.confirm(message)) {
        setBusy(null);
        return applyOne(file, true);
      }
      pushToast('error', t('modUpdates.failed'), message);
      return false;
    } finally {
      setBusy(null);
    }
  };

  const updateAll = async () => {
    setBusy(null);
    setUpdatingAll(true);
    let changed = false;
    try {
      for (const file of report?.files ?? []) {
        if (!file.updateAvailable) continue;
        changed = (await applyOne(file)) || changed;
      }
    } finally {
      setUpdatingAll(false);
      if (changed) await load();
    }
  };

  const updates = (report?.files ?? []).filter((file) => file.updateAvailable);

  return (
    <Modal
      open
      onClose={onClose}
      closeable={!busy && !updatingAll}
      title={t('modUpdates.title')}
      eyebrow={instance.name}
      description={t('modUpdates.body')}
      size="lg"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={() => void load()} disabled={loading || Boolean(busy) || updatingAll}>
            <RefreshCw size={15} className={loading ? 'spin' : ''} /> {t('modUpdates.check')}
          </button>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose} disabled={Boolean(busy) || updatingAll}>
              {t('common.close')}
            </button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={() => void updateAll()}
              disabled={loading || updatingAll || updates.length === 0}
            >
              {updatingAll ? <LoaderCircle size={15} className="spin" /> : <RefreshCw size={15} />}
              {updatingAll ? t('modUpdates.updating') : t('modUpdates.updateAll')}
            </button>
          </div>
        </>
      }
    >
      {loading ? (
        <div className="mod-updates-loading">
          <LoaderCircle size={17} className="spin" /> {t('modUpdates.checking')}
        </div>
      ) : error ? (
        <div className="inline-banner banner-error">
          <TriangleAlert size={16} />
          <div><strong>{t('modUpdates.failed')}</strong><span>{error}</span></div>
        </div>
      ) : report ? (
        <>
          <div className="mod-updates-summary">
            <span className="chip is-key">{t('modUpdates.summary', { identified: report.identified, updates: report.updates })}</span>
            {report.unidentified > 0 && <span className="chip chip-warn">{t('modUpdates.unidentified', { count: report.unidentified })}</span>}
          </div>
          {report.warnings.length > 0 && (
            <div className="inline-banner banner-warning">
              <TriangleAlert size={15} />
              <div><strong>{t('modUpdates.warning')}</strong><span>{report.warnings[0]}</span></div>
            </div>
          )}
          {updates.length === 0 ? (
            <div className="mod-updates-empty">
              <RefreshCw size={22} />
              <strong>{t('modUpdates.none')}</strong>
              <span>{t('modUpdates.noneBody')}</span>
            </div>
          ) : (
            <div className="mod-updates-list">
              {updates.map((file) => (
                <div className="mod-update-row" key={file.relativePath}>
                  <div className="mod-update-copy">
                    <strong>{file.name}</strong>
                    <span>{t('modUpdates.path')}: <code>{file.relativePath}</code></span>
                    <div className="chip-row">
                      <span className="chip">{file.source === 'curseforge' ? 'CurseForge' : 'Modrinth'}</span>
                      <span className="chip">{t('modUpdates.current')} {file.currentVersion || '—'}</span>
                      <span className="chip is-key">{t('modUpdates.latest')} {file.latestVersion || file.latestName || '—'}</span>
                    </div>
                  </div>
                  <button
                    type="button"
                    className="btn btn-primary btn-sm"
                    onClick={() => void applyOne(file).then((ok) => { if (ok) void load(); })}
                    disabled={Boolean(busy) || updatingAll}
                  >
                    {busy === file.relativePath ? <LoaderCircle size={14} className="spin" /> : null}
                    {busy === file.relativePath ? t('modUpdates.updating') : t('modUpdates.update')}
                  </button>
                </div>
              ))}
            </div>
          )}
          {(report.files ?? []).some((file) => !file.updateAvailable && file.identified && !file.error) && (
            <p className="field-hint">{t('modUpdates.upToDateHint')}</p>
          )}
          <p className="field-hint">{t('modUpdates.rollbackHint')}</p>
        </>
      ) : null}
    </Modal>
  );
}
