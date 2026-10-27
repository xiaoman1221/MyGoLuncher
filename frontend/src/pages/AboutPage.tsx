import {
  BookOpen,
  Boxes,
  ExternalLink,
  FolderOpen,
  Code2,
  Heart,
  Info,
  Scale,
  TerminalSquare,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { LogoMark } from '../components/LogoMark';
import { UpdatePanel } from '../components/UpdatePanel';
import { LicensePanel } from '../components/LicensePanel';

const repository = 'https://github.com/xiaoman1221/MyMinecraftGoLuncher';
const issues = repository + '/issues';
const licenseURL = 'https://www.gnu.org/licenses/agpl-3.0.html';

export function AboutPage() {
  const { bootstrap, t, openDataFolder, pushToast } = useApp();
  const version = bootstrap?.version || '1.0.3';
  const official = Boolean(bootstrap?.license.configured);

  const openLink = async (url: string) => {
    try {
      await api.openURL(url);
    } catch (reason) {
      pushToast('error', t('about.openFailed'), (reason as Error).message);
    }
  };

  return (
    <div className="page about-page">
      <PageHeader
        kicker="ABOUT"
        title={<>{t('about.title')} <span className="title-accent">/</span> MYGO</>}
        description={t('about.description')}
        actions={
          <button type="button" className="btn btn-ghost" onClick={() => void openDataFolder()}>
            <FolderOpen size={15} /> {t('about.openData')}
          </button>
        }
      />

      <div className="about-grid">
        <section className="panel about-hero">
          <LogoMark size={66} />
          <div className="about-hero-copy">
            <div className="eyebrow"><span className="eyebrow-dash" />MYGO LUNCHER</div>
            <h2>MyGoLuncher</h2>
            <p>{t('about.tagline')}</p>
            <div className="about-tags">
              <span className="chip"><Scale size={12} /> AGPL-3.0</span>
              <span className="chip"><Boxes size={12} /> {official ? t('about.official') : t('about.community')}</span>
              <span className="chip"><TerminalSquare size={12} /> Go + Wails</span>
            </div>
          </div>
          <div className="about-version-box">
            <span>{t('about.version')}</span>
            <strong>v{version}</strong>
            <small>{t('about.buildChannel')}</small>
          </div>
        </section>

        <div className="about-license-stack about-wide">
          <LicensePanel />
          <UpdatePanel />
        </div>

        <section className="panel about-panel">
          <div className="panel-head">
            <h2><Info size={16} /> {t('about.infoTitle')}</h2>
            <p>{t('about.infoBody')}</p>
          </div>
          <div className="about-facts">
            <div className="about-fact">
              <span>{t('about.version')}</span>
              <strong>v{version}</strong>
            </div>
            <div className="about-fact">
              <span>{t('about.buildChannel')}</span>
              <strong>{official ? t('about.officialBuild') : t('about.communityBuild')}</strong>
            </div>
            <div className="about-fact">
              <span>{t('about.dataDirectory')}</span>
              <code title={bootstrap?.paths.data}>{bootstrap?.paths.data || '—'}</code>
            </div>
            <div className="about-fact">
              <span>{t('about.openSource')}</span>
              <strong>AGPL-3.0</strong>
            </div>
          </div>
        </section>

        <section className="panel about-panel">
          <div className="panel-head">
            <h2><BookOpen size={16} /> {t('about.linksTitle')}</h2>
            <p>{t('about.linksBody')}</p>
          </div>
          <div className="about-links">
            <button type="button" className="about-link" onClick={() => void openLink(repository)}>
              <Code2 size={16} />
              <span><strong>{t('about.repository')}</strong><small>xiaoman1221/MyMinecraftGoLuncher</small></span>
              <ExternalLink size={14} />
            </button>
            <button type="button" className="about-link" onClick={() => void openLink(issues)}>
              <Info size={16} />
              <span><strong>{t('about.issues')}</strong><small>{t('about.issuesBody')}</small></span>
              <ExternalLink size={14} />
            </button>
            <button type="button" className="about-link" onClick={() => void openLink(licenseURL)}>
              <Scale size={16} />
              <span><strong>{t('about.licenseLink')}</strong><small>GNU AGPL-3.0</small></span>
              <ExternalLink size={14} />
            </button>
          </div>
        </section>

        <section className="panel about-panel about-wide">
          <div className="panel-head">
            <h2><Heart size={16} /> {t('about.techTitle')}</h2>
            <p>{t('about.techBody')}</p>
          </div>
          <div className="about-tech">
            <span>Go</span>
            <span>Wails v2</span>
            <span>React 19</span>
            <span>TypeScript</span>
            <span>Vite</span>
            <span>Lucide</span>
            <span>Mojang API</span>
            <span>Modrinth</span>
            <span>CurseForge</span>
          </div>
          <div className="about-license">
            <strong>{t('about.licenseTitle')}</strong>
            <p>{t('about.licenseBody')}</p>
          </div>
        </section>

        <section className="panel about-panel about-wide about-disclaimer">
          <div>
            <strong>{t('about.disclaimerTitle')}</strong>
            <p>{t('about.disclaimer')}</p>
          </div>
          <span className="about-copyright">© 2026 MyGoLuncher</span>
        </section>
      </div>
    </div>
  );
}
