import { useEffect, useState } from 'react';
import { Gift, KeyRound, LoaderCircle, Mail, RefreshCw, Send, ShieldCheck, TriangleAlert, Unlink, X } from 'lucide-react';
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
  const { t, theme, language, confirm } = useApp();
  const [code, setCode] = useState('');
  const [email, setEmail] = useState(status.email ?? '');
  const [customer, setCustomer] = useState(status.customer ?? '');
  const [verificationCode, setVerificationCode] = useState('');
  const [emailOpen, setEmailOpen] = useState(false);
  const [emailSent, setEmailSent] = useState(false);
  const [notice, setNotice] = useState('');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState('');
  const [current, setCurrent] = useState(status);

  useEffect(() => {
    setCurrent(status);
    setEmail((value) => value || status.email || '');
    setCustomer((value) => value || status.customer || '');
  }, [status]);

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
    setNotice('');
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
    const trimmedCode = code.trim();
    const trimmedEmail = email.trim();
    const trimmedCustomer = customer.trim();
    if (!trimmedCode) {
      setError(t('license.missingCode'));
      return;
    }
    if (!trimmedEmail) {
      setError(t('license.missingEmail'));
      return;
    }
    if (!trimmedCustomer) {
      setError(t('license.missingCustomer'));
      return;
    }
    void run('activate', () => api.activateLicense(trimmedCode, trimmedEmail, trimmedCustomer));
  };

  const validate = () => void run('validate', () => api.validateLicense());
  const startTrial = () => void run('trial', () => api.startTrial());

  const unbind = async () => {
    if (!current.canSelfUnbind) {
      setEmailOpen(true);
      setError(t('license.selfUnbindUnavailable'));
      return;
    }
    if (!(await confirm({ message: t('license.unbindConfirm'), danger: true }))) return;
    await run('unbind', () => api.unbindLicense());
  };

  const requestEmailUnbind = async () => {
    const value = email.trim();
    if (!value) {
      setError(t('license.missingEmail'));
      return;
    }
    setBusy('emailRequest');
    setError('');
    setNotice('');
    try {
      await api.requestEmailUnbind(value);
      setEmailSent(true);
      setVerificationCode('');
      setNotice(t('license.emailUnbindSent'));
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(null);
    }
  };

  const confirmEmailUnbind = async () => {
    const value = email.trim();
    if (!value) {
      setError(t('license.missingEmail'));
      return;
    }
    if (!/^\d{6}$/.test(verificationCode.trim())) {
      setError(t('license.verificationHint'));
      return;
    }
    setBusy('emailConfirm');
    setError('');
    setNotice('');
    try {
      const next = await api.confirmEmailUnbind(value, verificationCode.trim());
      setCurrent(next);
      setEmailSent(false);
      setVerificationCode('');
      setEmailOpen(false);
      setNotice(t('license.emailUnbindSuccess'));
      await onActivated();
      onClose?.();
    } catch (reason) {
      setError((reason as Error).message);
    } finally {
      setBusy(null);
    }
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
        {current.email && <p className="license-message">{t('license.activatedEmail', { email: current.email })}</p>}

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

        <div className="license-identity">
          <label className="field">
            <span className="field-label"><Mail size={14} /> {t('license.emailLabel')}</span>
            <input
              className="text-input"
              type="email"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              placeholder={t('license.emailPlaceholder')}
              autoComplete="email"
              disabled={Boolean(busy)}
            />
            <span className="field-hint">{t('license.emailHint')}</span>
          </label>
          <label className="field">
            <span className="field-label">{t('license.customerLabel')}</span>
            <input
              className="text-input"
              value={customer}
              onChange={(event) => setCustomer(event.target.value)}
              placeholder={t('license.customerPlaceholder')}
              autoComplete="organization"
              disabled={Boolean(busy)}
            />
            <span className="field-hint">{t('license.customerHint')}</span>
          </label>
        </div>

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
        {notice && <p className="license-message license-message-success">{notice}</p>}

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

        {emailOpen && (
          <section className="license-email-unbind">
            <div className="license-email-head">
              <Mail size={15} />
              <div>
                <strong>{t('license.emailUnbindTitle')}</strong>
                <span>{t('license.emailUnbindBody')}</span>
              </div>
            </div>
            <div className="license-email-actions">
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => void requestEmailUnbind()} disabled={Boolean(busy) || !current.activated}>
                {busy === 'emailRequest' ? <LoaderCircle size={14} className="spin" /> : <Send size={14} />}
                {busy === 'emailRequest' ? t('license.emailUnbindSending') : t('license.emailUnbindRequest')}
              </button>
              {emailSent && <span className="field-hint">{t('license.emailUnbindSent')}</span>}
            </div>
            {emailSent && (
              <>
                <label className="field">
                  <span className="field-label">{t('license.verificationLabel')}</span>
                  <input
                    className="text-input mono"
                    value={verificationCode}
                    onChange={(event) => setVerificationCode(event.target.value.replace(/\D/g, '').slice(0, 6))}
                    placeholder={t('license.verificationPlaceholder')}
                    inputMode="numeric"
                    autoComplete="one-time-code"
                    maxLength={6}
                    disabled={Boolean(busy)}
                  />
                  <span className="field-hint">{t('license.verificationHint')}</span>
                </label>
                <button type="button" className="btn btn-danger btn-sm" onClick={() => void confirmEmailUnbind()} disabled={Boolean(busy) || verificationCode.length !== 6}>
                  {busy === 'emailConfirm' ? <LoaderCircle size={14} className="spin" /> : <Unlink size={14} />}
                  {busy === 'emailConfirm' ? t('license.emailUnbindConfirming') : t('license.emailUnbindConfirm')}
                </button>
              </>
            )}
          </section>
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
          {current.activated && !current.trial && current.canSelfUnbind && (
            <button type="button" className="btn btn-ghost btn-danger-ghost" onClick={unbind} disabled={Boolean(busy) || !current.configured}>
              <Unlink size={15} /> {t('license.unbind')}
            </button>
          )}
          {current.activated && !current.trial && !emailOpen && (
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => setEmailOpen(true)} disabled={Boolean(busy)}>
              <Mail size={14} /> {t('license.emailUnbind')}
            </button>
          )}
        </div>

        <p className="license-note">{t('license.note')}</p>
      </main>
    </div>
  );
}
