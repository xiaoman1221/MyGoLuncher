import { useEffect, useState } from 'react';
import { LogOut, Minimize2, Power, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { Modal } from './Modal';

type CloseAction = 'minimize' | 'exit';

export function ExitConfirmModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { settings, instances, saveSettings, pushToast, t } = useApp();
  const [remember, setRemember] = useState(false);
  const [busy, setBusy] = useState<CloseAction | null>(null);
  const runningCount = instances.filter((instance) => instance.running).length;

  useEffect(() => {
    if (open) {
      setRemember(false);
      setBusy(null);
    }
  }, [open]);

  const choose = async (action: CloseAction) => {
    setBusy(action);
    try {
      if (remember && settings) {
        await saveSettings({ ...settings, closeBehavior: action });
      }
      if (action === 'exit') {
        await api.quitApp();
        return;
      }
      await api.minimizeWindow();
      onClose();
    } catch (reason) {
      pushToast('error', t('exit.failed'), (reason as Error).message);
    } finally {
      setBusy(null);
    }
  };

  return (
    <Modal
      open={open}
      onClose={busy ? () => undefined : onClose}
      title={t('exit.title')}
      eyebrow="EXIT"
      size="sm"
      footer={
        <div className="modal-actions modal-actions-end">
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={Boolean(busy)}>
            {t('common.cancel')}
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => void choose('minimize')} disabled={Boolean(busy)}>
            <Minimize2 size={15} />
            {busy === 'minimize' ? t('exit.minimizing') : t('exit.minimize')}
          </button>
          <button type="button" className="btn btn-danger" onClick={() => void choose('exit')} disabled={Boolean(busy)}>
            <Power size={15} />
            {busy === 'exit' ? t('exit.quitting') : t('exit.quit')}
          </button>
        </div>
      }
    >
      <div className="exit-dialog">
        <div className="exit-dialog-icon">
          {runningCount > 0 ? <TriangleAlert size={22} /> : <LogOut size={22} />}
        </div>
        <div className="exit-dialog-copy">
          <strong>{t('exit.body')}</strong>
          {runningCount > 0 && (
            <p className="exit-dialog-warning">
              {t('exit.running', { count: runningCount })}
            </p>
          )}
          <p>{t('exit.minimizeHint')}</p>
        </div>
      </div>
      <label className="checkbox-row exit-remember">
        <input type="checkbox" checked={remember} onChange={(event) => setRemember(event.target.checked)} />
        <span>
          <strong>{t('exit.remember')}</strong>
          <small>{t('exit.rememberHint')}</small>
        </span>
      </label>
    </Modal>
  );
}
