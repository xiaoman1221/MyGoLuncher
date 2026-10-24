import { BadgeCheck, FolderOpen, Moon, Sun, TerminalSquare, WifiOff } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { initials } from '../lib/format';
import type { PageKey } from '../lib/types';

const pageLabels: Record<PageKey, string> = {
  home: '首页',
  versions: '版本',
  settings: '设置',
};

export function TopBar() {
  const {
    page, settings, instances, isDemo, selectedAccount, openAccounts,
    openDataFolder, openConsole, theme, setTheme,
  } = useApp();
  const running = instances.filter((instance) => instance.running);
  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';
  const online = selectedAccount?.kind === 'microsoft' || selectedAccount?.kind === 'authlib';

  return (
    <div className="topbar">
      <div className="topbar-left">
        <span className="topbar-root">MYGO</span>
        <span className="topbar-slash">/</span>
        <span className="topbar-page">{pageLabels[page]}</span>
      </div>
      <div className="topbar-right">
        {isDemo && <span className="demo-pill">演示模式</span>}
        <span className={`topbar-mode ${online ? 'is-online' : ''}`}>
          {online ? <BadgeCheck size={13} /> : <WifiOff size={13} />}
          {selectedAccount?.label ?? '离线'}
        </span>
        {running.length > 0 && (
          <button type="button" className="topbar-console" onClick={() => openConsole(running[0].id)}>
            <span className="live-pulse" />
            <TerminalSquare size={15} />
            <span>控制台</span>
          </button>
        )}
        <button
          type="button"
          className="icon-button theme-toggle"
          onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')}
          title={theme === 'light' ? '切换到黑夜模式' : '切换到白天模式'}
          aria-label={theme === 'light' ? '切换到黑夜模式' : '切换到白天模式'}
        >
          {theme === 'light' ? <Moon size={17} /> : <Sun size={17} />}
        </button>
        <button type="button" className="icon-button" onClick={() => void openDataFolder()} title="打开游戏目录" aria-label="打开游戏目录">
          <FolderOpen size={17} />
        </button>
        <button type="button" className="topbar-player" onClick={openAccounts}>
          <div className="avatar avatar-xs">{initials(player)}</div>
          <span>{player}</span>
        </button>
      </div>
    </div>
  );
}
