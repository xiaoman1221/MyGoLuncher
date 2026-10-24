import { Boxes, FolderOpen, Home, LayoutList, PackageOpen, RadioTower, Server, Settings, Settings2, TerminalSquare } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { accountKindKey } from '../lib/i18n';
import { cn, initials } from '../lib/format';
import type { PageKey } from '../lib/types';
import { LogoMark } from './LogoMark';

const navItems: Array<{
  key: PageKey;
  labelKey: 'common.home' | 'common.versionDownload' | 'instanceSettings.title' | 'resources.title' | 'modpacks.title' | 'servers.title' | 'lan.title' | 'common.settings';
  icon: typeof Home;
}> = [
  { key: 'home', labelKey: 'common.home', icon: Home },
  { key: 'versions', labelKey: 'common.versionDownload', icon: Boxes },
  { key: 'instanceSettings', labelKey: 'instanceSettings.title', icon: Settings2 },
  { key: 'resources', labelKey: 'resources.title', icon: LayoutList },
  { key: 'modpacks', labelKey: 'modpacks.title', icon: PackageOpen },
  { key: 'servers', labelKey: 'servers.title', icon: Server },
  { key: 'lan', labelKey: 'lan.title', icon: RadioTower },
  { key: 'settings', labelKey: 'common.settings', icon: Settings },
];

export function Sidebar() {
  const {
    page, navigate, settings, instances, selectedAccount, openAccounts,
    openConsole, openDataFolder, bootstrap, lanPeerCount, t,
  } = useApp();
  const running = instances.filter((instance) => instance.running);
  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';
  const accountLabel = t(accountKindKey(selectedAccount?.kind ?? 'offline'));

  return (
    <aside className="sidebar">
      <button type="button" className="brand" onClick={() => navigate('home')} aria-label={t('nav.backHome')}>
        <LogoMark size={38} />
        <div className="brand-copy">
          <span className="brand-title">MYGO</span>
          <span className="brand-sub">LUNCHER · MINECRAFT</span>
        </div>
      </button>

      <nav className="sidebar-nav" aria-label={t('nav.main')}>
        {navItems.map((item) => {
          const Icon = item.icon;
          const active = page === item.key;
          return (
            <button
              key={item.key}
              type="button"
              className={cn('nav-item', active && 'active')}
              onClick={() => navigate(item.key)}
              aria-current={active ? 'page' : undefined}
            >
              <span className="nav-icon"><Icon size={18} strokeWidth={active ? 2.2 : 1.8} /></span>
              <span>{t(item.labelKey)}</span>
              {item.key === 'home' && instances.length > 0 && (
                <span className="nav-badge">{instances.length}</span>
              )}
              {item.key === 'lan' && lanPeerCount > 0 && (
                <span className="nav-badge">{lanPeerCount}</span>
              )}
            </button>
          );
        })}
      </nav>

      {running.length > 0 && (
        <button type="button" className="sidebar-live" onClick={() => openConsole(running[0].id)}>
          <span className="live-pulse" />
          <span>
            <strong>{t('sidebar.running', { count: running.length })}</strong>
            <small>{t('sidebar.openConsole')}</small>
          </span>
          <TerminalSquare size={17} />
        </button>
      )}

      <div className="sidebar-spacer" />

      <button type="button" className="sidebar-folder" onClick={() => void openDataFolder()}>
        <FolderOpen size={16} />
        <span>{t('common.openGameFolder')}</span>
      </button>

      <button type="button" className="profile-card" onClick={openAccounts} title={t('sidebar.switchAccount')}>
        <div className="avatar avatar-sm">{initials(player)}</div>
        <div className="profile-copy">
          <strong>{player}</strong>
          <span>{accountLabel}</span>
        </div>
        <span className={`profile-status kind-${selectedAccount?.kind ?? 'offline'}`} />
      </button>

      <div className="sidebar-foot">
        <span>v{bootstrap?.version ?? '1.0.0'}</span>
        <span className="foot-dot" />
        <span>MYGO</span>
      </div>
    </aside>
  );
}
