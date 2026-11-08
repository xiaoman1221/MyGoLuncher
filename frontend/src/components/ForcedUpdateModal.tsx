import { useEffect, useState } from 'react';
import { Download, LoaderCircle, LogOut, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import type { UpdateInfo, UpdateProgress } from '../lib/types';
import { Modal } from './Modal';
import { UpdateProgressBar } from './UpdateProgressBar';

// A forced OTA update (tag=force) must not be dismissible. The launcher stays
// usable only after the update is applied or the user explicitly quits.
export function ForcedUpdateModal({ info }: { info: UpdateInfo }) {
  const { t, pushToast } = useApp();
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [progress, setProgress] = useState<UpdateProgress | null>(null);

  useEffect(() => onEvent<UpdateProgress>('update:progress', (next) => setProgress(next)), []);

  const apply = async () => {
    setBusy(true);
    setError('');
    setProgress(null);
    try {
      if (info.hasApp) {
        await api.applyAppUpdate();
        return;
      }
      if (info.hasWeb) {
        await api.applyWebUpdate();
        pushToast('success', t('settings.updateWebApplied'));
        return;
      }
      setError(t('settings.updateForceUnavailable'));
      setBusy(false);
    } catch (reason) {
      setProgress(null);
      setError((reason as Error).message);
      setBusy(false);
    }
  };

  const quit = async () => {
    try {
      await api.quitApp();
    } catch (reason) {
      setError((reason as Error).message);
    }
  };

  const version = info.version ?? info.appVersion ?? info.webVersion ?? '';

  return (
    <Modal
      open
      onClose={() => undefined}
      closeable={false}
      title={t('settings.updateForceTitle')}
      description={t('settings.updateForceBody', { version })}
      size="md"
      overlayClassName="modal-layer-forced"
      footer={
        <div className="modal-actions modal-actions-end">
          <button type="button" className="btn btn-ghost" onClick={() => void quit()} disabled={busy}>
            <LogOut size={15} /> {t('settings.updateForceQuit')}
          </button>
          <button type="button" className="btn btn-primary" onClick={() => void apply()} disabled={busy || (!info.hasApp && !info.hasWeb)}>
            {busy ? <LoaderCircle size={15} className="spin" /> : <Download size={15} />}
            {busy ? t('settings.updateApplying') : t('settings.updateForceNow')}
          </button>
        </div>
      }
    >
      <div className="forced-update-body">
        {info.notes && <p className="update-notes">{info.notes}</p>}
        {progress && <UpdateProgressBar progress={progress} />}
        {error && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div><strong>{t('settings.updateFailed')}</strong><span>{error}</span></div>
          </div>
        )}
      </div>
    </Modal>
  );
}
