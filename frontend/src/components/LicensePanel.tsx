import { KeyRound, ShieldCheck } from 'lucide-react';
import { useApp } from '../lib/app-context';

export function LicensePanel() {
  const { t, bootstrap, openLicense } = useApp();
  const status = bootstrap?.license;
  const active = status?.state === 'active' || status?.state === 'grace' || status?.state === 'trial';

  return (
    <section className="panel">
      <div className="panel-head">
        <h2>{t('settings.licenseTitle')}</h2>
        <p>{t('settings.licenseBody')}</p>
      </div>
      <div className="settings-fields">
        <div className="license-summary">
          <div>
            <span className="field-label"><ShieldCheck size={14} /> {t('settings.licenseStatus')}</span>
            <strong className={active ? 'license-summary-active' : ''}>
              {status?.state === 'trial'
                ? t('license.trialActive')
                : status?.state === 'trial_expired'
                  ? t('license.trialExpired')
                  : active ? t('license.active') : t('license.inactive')}
            </strong>
            {status?.state === 'trial' && status.trialDaysLeft != null && (
              <span className="field-hint">{t('license.trialDaysLeft', { days: status.trialDaysLeft })}</span>
            )}
            {status && status.state !== 'active' && status.state !== 'grace' && (status.trialRemaining ?? 0) > 0 && (
              <span className="field-hint">
                {t('license.trialRemaining', { count: status.trialRemaining ?? 0, total: status.trialDays ?? 7 })}
              </span>
            )}
            {status?.code && <code>{status.code}</code>}
            {status?.expiresAt && (
              <span className="field-hint">{t('license.expires', { date: new Date(status.expiresAt).toLocaleString() })}</span>
            )}
            {status?.message && <span className="field-hint">{status.message}</span>}
            {status && !status.configured && <span className="field-hint">{t('license.communityHint')}</span>}
          </div>
          <button type="button" className="btn btn-primary btn-sm" onClick={openLicense}>
            <KeyRound size={14} /> {active ? t('settings.licenseManage') : t('settings.licenseActivate')}
          </button>
        </div>
        <div className="license-feature-list">
          {t('settings.licenseFree')}
          <span>{t('settings.licensePaidList')}</span>
        </div>
      </div>
    </section>
  );
}
