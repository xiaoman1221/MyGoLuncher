import { useState } from 'react';
import { LoaderCircle, LogOut, ShieldAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { LogoMark } from './LogoMark';

interface ElevationGateProps {
  onConfirmed: () => Promise<void>;
}

export function ElevationGate({ onConfirmed }: ElevationGateProps) {
  const { t, theme } = useApp();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');

  const continueRun = async () => {
    setBusy('continue');
    setError('');
    try {
      await api.confirmElevation(true);
      await onConfirmed();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const exitApp = async () => {
    setBusy('exit');
    setError('');
    try {
      await api.confirmElevation(false);
    } catch (reason) {
      setError((reason as Error).message);
      setBusy(null);
    }
  };

  return (
    <div className="license-screen elevation-screen" data-theme={theme}>
      <div className="license-backdrop" aria-hidden="true" />
      <main className="license-card elevation-card">
        <div className="license-brand">
          <LogoMark size={48} />
          <div>
            <strong>MYGO</strong>
            <span>SECURITY NOTICE</span>
          </div>
        </div>
        <div className="elevation-icon"><ShieldAlert size={28} /></div>
        <div className="license-heading">
          <span className="eyebrow"><span className="eyebrow-dash" />{t('elevation.eyebrow')}</span>
          <h1>{t('elevation.title')}</h1>
          <p>{t('elevation.body')}</p>
        </div>
        <div className="elevation-points">
          <span>{t('elevation.pointOne')}</span>
          <span>{t('elevation.pointTwo')}</span>
          <span>{t('elevation.pointThree')}</span>
        </div>
        {error && <p className="license-message">{error}</p>}
        <div className="license-actions">
          <button type="button" className="btn btn-primary" onClick={() => void continueRun()} disabled={Boolean(busy)}>
            {busy === 'continue' ? <LoaderCircle size={16} className="spin" /> : null}
            {t('elevation.continue')}
          </button>
          <button type="button" className="btn btn-ghost" onClick={() => void exitApp()} disabled={Boolean(busy)}>
            {busy === 'exit' ? <LoaderCircle size={15} className="spin" /> : <LogOut size={15} />}
            {t('elevation.exit')}
          </button>
        </div>
        <p className="license-note">{t('elevation.note')}</p>
      </main>
    </div>
  );
}
