import { useEffect, useState } from 'react';
import { Gift, KeyRound, LoaderCircle, RefreshCw, ShieldCheck, TriangleAlert, Unlink, X } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import type { LicenseStatus } from '../lib/types';
import { LogoMark } from './LogoMark';

interface LicenseGateProps {
  status: LicenseStatus;
  onActivated: () => Promise<void>;
  onClose?: () => void;
}

export function LicenseGate({ status, onActivated, onClose }: LicenseGateProps) {
  const { t, theme, language } = useApp();
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [current, setCurrent] = useState(status);

  useEffect(() => setCurrent(status), [status]);

  const stateLabel = (value: LicenseStatus) => {
    switch (value.state) {
      case 'active': return t('license.active');
      case 'grace': return t('license.grace');
      case 'expired': return t('license.expired');
      case 'invalid': return t('license.invalid');
      case 'error': return t('license.error');
      case 'machine_changed': return t('license.machineChanged');
      case 'trial': return t('license.trialActive');
      case 'trial_expired': return t('license.trialExpired');
      default: return t('license.inactive');
    }
  };

  const run = async (action: string, work: () => Promise<LicenseStatus>) => {
    setBusy(action);
    setError('');
    try {
      const next = await work();
      setCurrent(next);
      if (next.state === 'active' || next.state === 'grace' || next.state === 'trial') {
        await onActivated();
        onClose?.();
      } else if (next.message) {
        setError(next.message);
      }
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const activate = () => {
    if (!code.trim()) {
      setError(t('license.missingCode'));
      return;
    }
    void run('activate', () => api.activateLicense(code.trim()));
  };

  const validate = () => void run('validate', () => api.validateLicense());
  const startTrial = () => void run('trial', () => api.startTrial());

  const unbind = () => {
    if (!window.confirm(t('license.unbindConfirm'))) return;
    void run('unbind', () => api.unbindLicense());
  };

  const trialRemaining = current.trialRemaining ?? 3;
  const canStartTrial = current.configured
    && trialRemaining > 0
    && !['active', 'grace', 'trial'].includes(current.state);

  return (
    <div className="license-screen" data-theme={theme}>
      <div className="license-backdrop" aria-hidden="true" />
      <main className="license-card">
        {onClose && (
          <button type="button" className="license-close" onClick={onClose} aria-label={t('common.close')}>
            <X size={16} />
          </button>
        )}
        <div className="license-brand">
          <LogoMark size={48} />
          <div>
            <strong>MYGO</strong>
            <span>LICENSE ACTIVATION</span>
          </div>
        </div>
        <div className="license-heading">
          <span className="eyebrow"><span className="eyebrow-dash" />{t('license.eyebrow')}</span>
          <h1>{t('license.title')}</h1>
          <p>{t('license.body')}</p>
        </div>

        <div className={'license-state is-' + current.state}>
          <ShieldCheck size={16} />
          <span>{stateLabel(current)}</span>
          {current.code && <code>{current.code}</code>}
        </div>

        <div className="license-machine">
          <span>{t('license.machineCode')}</span>
          <code>{current.machineCode || '—'}</code>
          <small>{t('license.machineHint')}</small>
        </div>

        <label className="field">
          <span className="field-label"><KeyRound size={14} /> {t('license.codeLabel')}</span>
          <input
            className="text-input mono"
            value={code}
            onChange={(event) => setCode(event.target.value)}
            onKeyDown={(event) => {
              if (event.nativeEvent.isComposing || event.keyCode === 229) return;
              if (event.key === 'Enter') activate();
            }}
            placeholder={t('license.codePlaceholder')}
            autoComplete="off"
            spellCheck={false}
            disabled={Boolean(busy)}
          />
          <span className="field-hint">{t('license.codeHint')}</span>
        </label>

        {!current.configured && (
          <div className="inline-banner banner-warning">
            <TriangleAlert size={16} />
            <div><strong>{t('license.community')}</strong><span>{t('license.communityHint')}</span></div>
          </div>
        )}

        {error && (
          <div className="inline-banner banner-error">
            <TriangleAlert size={16} />
            <div><strong>{t('license.failed')}</strong><span>{error}</span></div>
          </div>
        )}

        {current.message && !error && current.state !== 'inactive' && (
          <p className="license-message">{current.message}</p>
        )}
        {current.state === 'trial' && current.trialDaysLeft != null && (
          <p className="license-message">{t('license.trialDaysLeft', { days: current.trialDaysLeft })}</p>
        )}
        {current.state === 'trial_expired' && (
          <p className="license-message">{t('license.trialExpiredHint')}</p>
        )}
        {current.configured && current.state !== 'active' && current.state !== 'grace' && (
          <p className="license-message">{t('license.trialRemaining', {
            count: trialRemaining,
            total: current.trialDays ?? 7,
          })}</p>
        )}
        {current.expiresAt && (
          <p className="license-message">{t('license.expires', { date: new Date(current.expiresAt).toLocaleString(language) })}</p>
        )}
        {current.features.length > 0 && (
          <p className="license-message">{t('license.features', { features: current.features.join(' · ') })}</p>
        )}

        <div className="license-actions">
          {canStartTrial && (
            <button type="button" className="btn btn-ghost" onClick={startTrial} disabled={Boolean(busy)}>
              {busy === 'trial' ? <LoaderCircle size={16} className="spin" /> : <Gift size={16} />}
              {busy === 'trial' ? t('license.trialStarting') : t('license.trialStart', { days: current.trialDays ?? 7 })}
            </button>
          )}
          <button type="button" className="btn btn-primary" onClick={activate} disabled={Boolean(busy) || !current.configured}>
            {busy === 'activate' ? <LoaderCircle size={16} className="spin" /> : <KeyRound size={16} />}
            {busy === 'activate' ? t('license.activating') : t('license.activate')}
          </button>
          <button type="button" className="btn btn-ghost" onClick={validate} disabled={Boolean(busy) || !current.activated || !current.configured || current.state === 'trial'}>
            {busy === 'validate' ? <LoaderCircle size={15} className="spin" /> : <RefreshCw size={15} />}
            {busy === 'validate' ? t('license.validating') : t('license.validate')}
          </button>
          {current.activated && !current.trial && (
            <button type="button" className="btn btn-ghost btn-danger-ghost" onClick={unbind} disabled={Boolean(busy) || !current.configured}>
              <Unlink size={15} /> {t('license.unbind')}
            </button>
          )}
        </div>

        <p className="license-note">{t('license.note')}</p>
      </main>
    </div>
  );
}
