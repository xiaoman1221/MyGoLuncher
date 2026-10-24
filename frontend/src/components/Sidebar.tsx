import { Boxes, FolderOpen, Home, Settings, TerminalSquare } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { cn, initials } from '../lib/format';
import type { PageKey } from '../lib/types';
import { LogoMark } from './LogoMark';

const navItems: Array<{ key: PageKey; label: string; icon: typeof Home }> = [
  { key: 'home', label: '首页', icon: Home },
  { key: 'versions', label: '版本', icon: Boxes },
  { key: 'settings', label: '设置', icon: Settings },
];

export function Sidebar() {
  const {
    page, navigate, settings, instances, selectedAccount, openAccounts,
    openConsole, openDataFolder, bootstrap,
  } = useApp();
  const running = instances.filter((instance) => instance.running);
  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';

  return (
    <aside className="sidebar">
      <button type="button" className="brand" onClick={() => navigate('home')} aria-label="返回首页">
        <LogoMark size={38} />
        <div className="brand-copy">
          <span className="brand-title">MYGO</span>
          <span className="brand-sub">LUNCHER · MINECRAFT</span>
        </div>
      </button>

      <nav className="sidebar-nav" aria-label="主导航">
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
              <span>{item.label}</span>
              {item.key === 'home' && instances.length > 0 && (
                <span className="nav-badge">{instances.length}</span>
              )}
            </button>
          );
        })}
      </nav>

      {running.length > 0 && (
        <button type="button" className="sidebar-live" onClick={() => openConsole(running[0].id)}>
          <span className="live-pulse" />
          <span>
            <strong>{running.length} 个版本运行中</strong>
            <small>打开游戏控制台</small>
          </span>
          <TerminalSquare size={17} />
        </button>
      )}

      <div className="sidebar-spacer" />

      <button type="button" className="sidebar-folder" onClick={() => void openDataFolder()}>
        <FolderOpen size={16} />
        <span>打开游戏目录</span>
      </button>

      <button type="button" className="profile-card" onClick={openAccounts} title="切换账户">
        <div className="avatar avatar-sm">{initials(player)}</div>
        <div className="profile-copy">
          <strong>{player}</strong>
          <span>{selectedAccount?.label ?? '离线'}</span>
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
