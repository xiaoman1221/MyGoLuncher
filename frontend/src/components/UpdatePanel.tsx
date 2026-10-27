import { useState } from 'react';
import { Download, LoaderCircle, RefreshCw, RotateCcw, Sparkles, TriangleAlert, Zap } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import type { UpdateInfo } from '../lib/types';

export function UpdatePanel({ className = '' }: { className?: string }) {
  const { bootstrap, t, pushToast } = useApp();
  const [info, setInfo] = useState<UpdateInfo | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');

  const runningGames = (bootstrap?.instances ?? []).filter((instance) => instance.running).length;
  const canUpdate = () => {
    if (runningGames > 0) {
      pushToast('error', t('settings.updateGameRunning'));
      return false;
    }
    return true;
  };

  const run = async (action: string, task: () => Promise<string | void>) => {
    setBusy(action);
    setError('');
    try {
      await task();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const check = () => void run('check', async () => {
    const next = await api.checkForUpdates();
    setInfo(next);
    if (!next.available) pushToast('info', t('settings.updateUpToDate'));
  });

  const oneClick = () => void run('oneClick', async () => {
    const next = await api.checkForUpdates();
    setInfo(next);
    if (!next.available) {
      pushToast('info', t('settings.updateUpToDate'));
      return;
    }
    if (next.hasApp) {
      if (!canUpdate() || !window.confirm(t('settings.updateAppConfirm'))) return;
      await api.applyAppUpdate();
      return;
    }
    if (next.hasWeb) {
      if (!canUpdate()) return;
      await api.applyWebUpdate();
      pushToast('success', t('settings.updateWebApplied'));
      return;
    }
    pushToast('info', t('settings.updateUpToDate'));
  });

  const applyWeb = () => {
    if (!canUpdate()) return;
    void run('web', async () => {
      await api.applyWebUpdate();
      pushToast('success', t('settings.updateWebApplied'));
    });
  };

  const applyApp = () => {
    if (!canUpdate() || !window.confirm(t('settings.updateAppConfirm'))) return;
    void run('app', async () => {
      await api.applyAppUpdate();
    });
  };

  const rollback = () => {
    if (!window.confirm(t('settings.updateRollbackConfirm'))) return;
    void run('rollback', async () => {
      await api.rollbackWebUpdate();
    });
  };

  return (
    <section className={className ? 'panel ' + className : 'panel'}>
      <div className="panel-head">
        <h2>{t('settings.updateTitle')}</h2>
        <p>{t('settings.updateBody')}</p>
      </div>

      <div className="settings-fields">
        {runningGames > 0 && (
          <div className="inline-banner banner-warning">
            <TriangleAlert size={16} />
            <div><strong>{t('settings.updateGameRunning')}</strong><span>{t('settings.updateGameRunningBody')}</span></div>
          </div>
        )}
        <div className="update-row">
          <div>
            <strong>{t('settings.updateCurrent', { version: info?.currentVersion ?? '1.0.3' })}</strong>
            {info?.configured && info.available && (
              <span className="field-hint">
                {t('settings.updateAvailable', { version: info.version ?? info.appVersion ?? info.webVersion ?? '' })}
              </span>
            )}
            {info?.notes && <p className="update-notes">{info.notes}</p>}
          </div>
          <div className="update-row-actions">
            <button type="button" className="btn btn-ghost btn-sm" onClick={check} disabled={Boolean(busy)}>
              {busy === 'check' ? <LoaderCircle size={14} className="spin" /> : <RefreshCw size={14} />}
              {busy === 'check' ? t('settings.updateChecking') : t('settings.updateCheck')}
            </button>
            <button type="button" className="btn btn-primary btn-sm" onClick={oneClick} disabled={Boolean(busy)}>
              {busy === 'oneClick' ? <LoaderCircle size={14} className="spin" /> : <Zap size={14} />}
              {busy === 'oneClick' ? t('settings.updateApplying') : t('settings.updateOneClick')}
            </button>
          </div>
        </div>

        {info?.available && (
          <div className="update-actions">
            {info.hasWeb && (
              <button type="button" className="btn btn-primary btn-sm" onClick={applyWeb} disabled={Boolean(busy)}>
                {busy === 'web' ? <LoaderCircle size={15} className="spin" /> : <Sparkles size={15} />}
                {t('settings.updateWebAction', { version: info.webVersion ?? '' })}
              </button>
            )}
            {info.hasApp && (
              <button type="button" className="btn btn-primary btn-sm" onClick={applyApp} disabled={Boolean(busy)}>
                {busy === 'app' ? <LoaderCircle size={15} className="spin" /> : <Download size={15} />}
                {t('settings.updateAppAction', { version: info.appVersion ?? '' })}
              </button>
            )}
            <button type="button" className="btn btn-ghost btn-sm" onClick={rollback} disabled={Boolean(busy)}>
              {busy === 'rollback' ? <LoaderCircle size={14} className="spin" /> : <RotateCcw size={14} />}
              {t('settings.updateRollback')}
            </button>
          </div>
        )}

        {info?.mandatory && <p className="license-message">{t('settings.updateMandatory')}</p>}
        {info?.force && <p className="license-message">{t('settings.updateForce')}</p>}
        {error && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div><strong>{t('settings.updateFailed')}</strong><span>{error}</span></div>
          </div>
        )}
        <p className="field-hint">{t('settings.updateHint')}</p>
      </div>
    </section>
  );
}
