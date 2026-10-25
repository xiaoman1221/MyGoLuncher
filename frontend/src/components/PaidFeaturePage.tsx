import { LockKeyhole, ServerCog, UsersRound } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { PageHeader } from './PageHeader';

interface PaidFeaturePageProps {
  feature: 'server' | 'lan';
}

const meta = {
  server: {
    icon: ServerCog,
    kicker: 'PRO · SERVER',
    titleKey: 'paid.serverTitle' as const,
    bodyKey: 'paid.serverBody' as const,
    points: ['paid.serverPointOne', 'paid.serverPointTwo', 'paid.serverPointThree'] as const,
  },
  lan: {
    icon: UsersRound,
    kicker: 'PRO · LAN',
    titleKey: 'paid.lanTitle' as const,
    bodyKey: 'paid.lanBody' as const,
    points: ['paid.lanPointOne', 'paid.lanPointTwo', 'paid.lanPointThree'] as const,
  },
};

export function PaidFeaturePage({ feature }: PaidFeaturePageProps) {
  const { t, openLicense, bootstrap } = useApp();
  const license = bootstrap?.license;
  const info = meta[feature];
  const Icon = info.icon;

  return (
    <div className="page">
      <PageHeader kicker={info.kicker} title={t(info.titleKey)} description={t(info.bodyKey)} />
      <section className="paid-feature-card">
        <div className="paid-feature-icon"><Icon size={28} /></div>
        <div className="paid-feature-copy">
          <span className="eyebrow"><span className="eyebrow-dash" />{t('paid.proEyebrow')}</span>
          <h2>{t('paid.proTitle')}</h2>
          <p>{t('paid.proBody')}</p>
          <ul>
            {info.points.map((key) => <li key={key}>{t(key)}</li>)}
          </ul>
          <div className="paid-feature-actions">
            <button type="button" className="btn btn-primary" onClick={openLicense}>
              <LockKeyhole size={15} /> {t('paid.activate')}
            </button>
            {(license?.state === 'active' || license?.state === 'grace' || license?.state === 'trial') && license.features.length > 0 && (
              <span className="field-hint">{t('paid.currentFeatures', { features: license.features.join(' · ') })}</span>
            )}
          </div>
        </div>
      </section>
    </div>
  );
}
