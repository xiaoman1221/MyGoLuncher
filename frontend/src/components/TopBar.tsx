import { useCallback, useEffect, useState } from 'react';
import {
  BadgeCheck,
  Copy,
  FolderOpen,
  Minus,
  Moon,
  Square,
  Sun,
  TerminalSquare,
  WifiOff,
  X,
} from 'lucide-react';
import { useApp } from '../lib/app-context';
import { accountKindKey, type MessageKey } from '../lib/i18n';
import { initials } from '../lib/format';
import type { PageKey } from '../lib/types';

const pageKeys: Record<PageKey, MessageKey> = {
  home: 'topbar.page.home',
  versions: 'common.versionDownload',
  instanceSettings: 'topbar.page.instanceSettings',
  resources: 'topbar.page.resources',
  modpacks: 'topbar.page.modpacks',
  servers: 'topbar.page.servers',
  lan: 'topbar.page.lan',
  settings: 'topbar.page.settings',
};

type WailsRuntime = {
  WindowMinimise?: () => void;
  WindowToggleMaximise?: () => void;
  WindowIsMaximised?: () => Promise<boolean>;
  Quit?: () => void;
};

function wailsRuntime(): WailsRuntime | undefined {
  return (window as Window & { runtime?: WailsRuntime }).runtime;
}

export function TopBar() {
  const {
    page, settings, instances, isDemo, selectedAccount, openAccounts,
    openDataFolder, openConsole, theme, setTheme, t,
  } = useApp();
  const [maximised, setMaximised] = useState(false);
  const running = instances.filter((instance) => instance.running);
  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';
  const online = selectedAccount?.kind === 'microsoft' || selectedAccount?.kind === 'authlib';

  const syncMaximised = useCallback(() => {
    const rt = wailsRuntime();
    if (!rt?.WindowIsMaximised) return;
    rt.WindowIsMaximised().then(setMaximised).catch(() => {});
  }, []);

  useEffect(() => {
    if (isDemo) return;
    syncMaximised();
    window.addEventListener('resize', syncMaximised);
    return () => window.removeEventListener('resize', syncMaximised);
  }, [isDemo, syncMaximised]);

  const minimise = () => wailsRuntime()?.WindowMinimise?.();
  const toggleMaximise = () => {
    wailsRuntime()?.WindowToggleMaximise?.();
    window.setTimeout(syncMaximised, 160);
  };
  const close = () => wailsRuntime()?.Quit?.();

  return (
    <div
      className="topbar"
      onDoubleClick={(event) => {
        if (isDemo) return;
        if ((event.target as HTMLElement).closest('button')) return;
        toggleMaximise();
      }}
    >
      <div className="topbar-left">
        <span className="topbar-root">MYGO</span>
        <span className="topbar-slash">/</span>
        <span className="topbar-page">{t(pageKeys[page])}</span>
      </div>
      <div className="topbar-right">
        {isDemo && <span className="demo-pill">{t('app.demoMode')}</span>}
        <span className={`topbar-mode ${online ? 'is-online' : ''}`}>
          {online ? <BadgeCheck size={13} /> : <WifiOff size={13} />}
          {t(accountKindKey(selectedAccount?.kind ?? 'offline'))}
        </span>
        {running.length > 0 && (
          <button type="button" className="topbar-console" onClick={() => openConsole(running[0].id)}>
            <span className="live-pulse" />
            <TerminalSquare size={15} />
            <span>{t('topbar.console')}</span>
          </button>
        )}
        <button
          type="button"
          className="icon-button theme-toggle"
          onClick={() => setTheme(theme === 'light' ? 'dark' : 'light')}
          title={theme === 'light' ? t('topbar.toDark') : t('topbar.toLight')}
          aria-label={theme === 'light' ? t('topbar.toDark') : t('topbar.toLight')}
        >
          {theme === 'light' ? <Moon size={17} /> : <Sun size={17} />}
        </button>
        <button type="button" className="icon-button" onClick={() => void openDataFolder()} title={t('common.openGameFolder')} aria-label={t('common.openGameFolder')}>
          <FolderOpen size={17} />
        </button>
        <button type="button" className="topbar-player" onClick={openAccounts}>
          <div className="avatar avatar-xs">{initials(player)}</div>
          <span>{player}</span>
        </button>

        {!isDemo && (
          <div className="window-controls">
            <button type="button" className="window-control" onClick={minimise} title={t('topbar.minimize')} aria-label={t('topbar.minimize')}>
              <Minus size={15} />
            </button>
            <button
              type="button"
              className="window-control"
              onClick={toggleMaximise}
              title={maximised ? t('topbar.restore') : t('topbar.maximize')}
              aria-label={maximised ? t('topbar.restoreWindow') : t('topbar.maximizeWindow')}
            >
              {maximised ? <Copy size={13} /> : <Square size={13} />}
            </button>
            <button type="button" className="window-control is-close" onClick={close} title={t('topbar.close')} aria-label={t('topbar.closeWindow')}>
              <X size={16} />
            </button>
          </div>
        )}
      </div>
    </div>
  );
}
