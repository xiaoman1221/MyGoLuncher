import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { RefreshCw, TriangleAlert } from 'lucide-react';
import { api, isDemo } from './lib/api';
import { onEvent } from './lib/events';
import {
  AppContext,
  type ConfirmOptions,
  type Toast,
  type ToastAction,
  type ToastKind,
} from './lib/app-context';
import {
  LANGUAGE_STORAGE_KEY,
  detectSystemLanguage,
  localeFor,
  normalizeLanguage,
  resolveStartupLanguage,
  translate,
  type Language,
  type MessageKey,
} from './lib/i18n';
import type {
  AccountLoginEvent,
  AccountState,
  Bootstrap,
  InstanceView,
  LanSnapshot,
  LaunchState,
  LicenseStatus,
  PageKey,
  Settings,
  Theme,
} from './lib/types';
import { Sidebar } from './components/Sidebar';
import { TopBar } from './components/TopBar';
import { TaskTray } from './components/TaskTray';
import { ToastStack } from './components/ToastStack';
import { ConsoleDrawer } from './components/ConsoleDrawer';
import { LogoMark } from './components/LogoMark';
import { FunLine } from './components/FunLine';
import { CreateInstanceModal } from './components/CreateInstanceModal';
import { EditInstanceModal } from './components/EditInstanceModal';
import { ConfirmDeleteModal } from './components/ConfirmDeleteModal';
import { ConfirmDialog } from './components/ConfirmDialog';
import { AccountModal } from './components/AccountModal';
import { LaunchOverlay } from './components/LaunchOverlay';
import { LicenseGate } from './components/LicenseGate';
import { ElevationGate } from './components/ElevationGate';
import { ExitConfirmModal } from './components/ExitConfirmModal';
import { PaidFeaturePage } from './components/PaidFeaturePage';
import { HomePage } from './pages/HomePage';
import { VersionsPage } from './pages/VersionsPage';
import { InstanceSettingsPage } from './pages/InstanceSettingsPage';
import { ResourcesPage } from './pages/ResourcesPage';
import { ModpacksPage } from './pages/ModpacksPage';
import { ServersPage } from './pages/ServersPage';
import { LanPage } from './pages/LanPage';
import { SettingsPage } from './pages/SettingsPage';
import { AboutPage } from './pages/AboutPage';

const THEME_KEY = 'mygoluncher-theme';
const SELECTED_KEY = 'mygoluncher-selected';

export default function App() {
  const [bootstrap, setBootstrap] = useState<Bootstrap | null>(null);
  const [bootError, setBootError] = useState('');
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState<PageKey>('home');
  const [theme, setThemeState] = useState<Theme>(() => (
    window.localStorage.getItem(THEME_KEY) === 'light' ? 'light' : 'dark'
  ));
  const [language, setLanguageState] = useState<Language>(() => {
    const cached = window.localStorage.getItem(LANGUAGE_STORAGE_KEY);
    return cached ? normalizeLanguage(cached) : detectSystemLanguage();
  });
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [lanPeerCount, setLanPeerCount] = useState(0);
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [consoleId, setConsoleId] = useState<string | null>(null);
  const [createOpen, setCreateOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [accountsOpen, setAccountsOpen] = useState(false);
  const [launchingId, setLaunchingId] = useState<string | null>(null);
  const [launchPhase, setLaunchPhase] = useState<'preparing' | 'waiting'>('preparing');
  const [licenseOpen, setLicenseOpen] = useState(false);
  const [exitOpen, setExitOpen] = useState(false);
  const [confirmOptions, setConfirmOptions] = useState<ConfirmOptions | null>(null);
  const confirmResolver = useRef<((value: boolean) => void) | null>(null);
  const licenseActive = ['active', 'grace', 'trial'].includes(bootstrap?.license?.state ?? '');
  const elevationBlocked = Boolean(bootstrap?.elevated && !bootstrap.elevationConfirmed);

  const toastId = useRef(0);
  const toastTimers = useRef(new Map<number, number>());
  const settingsRef = useRef<Settings | null>(null);
  const instancesRef = useRef<InstanceView[]>([]);

  useEffect(() => {
    settingsRef.current = bootstrap?.settings ?? null;
  }, [bootstrap?.settings]);

  useEffect(() => {
    instancesRef.current = bootstrap?.instances ?? [];
  }, [bootstrap?.instances]);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    window.localStorage.setItem(THEME_KEY, next);
    document.documentElement.dataset.theme = next;
    document.documentElement.style.colorScheme = next;
  }, []);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    document.documentElement.style.colorScheme = theme;
  }, [theme]);

  const setLanguage = useCallback((next: Language) => {
    setLanguageState(next);
    window.localStorage.setItem(LANGUAGE_STORAGE_KEY, next);
    document.documentElement.lang = next;
  }, []);

  useEffect(() => {
    document.documentElement.lang = language;
  }, [language]);

  const t = useCallback(
    (key: MessageKey, vars?: Record<string, string | number>) => translate(language, key, vars),
    [language],
  );
  const locale = localeFor(language);

  const pushToast = useCallback((kind: ToastKind, title: string, message?: string, action?: ToastAction) => {
    const id = ++toastId.current;
    setToasts((current) => [...current.slice(-4), { id, kind, title, message, action }]);
    const timer = window.setTimeout(() => {
      setToasts((current) => current.filter((toast) => toast.id !== id));
      toastTimers.current.delete(id);
    }, kind === 'error' ? 6500 : 4200);
    toastTimers.current.set(id, timer);
  }, []);

  const dismissToast = useCallback((id: number) => {
    const timer = toastTimers.current.get(id);
    if (timer) window.clearTimeout(timer);
    toastTimers.current.delete(id);
    setToasts((current) => current.filter((toast) => toast.id !== id));
  }, []);

  // In-app confirmations avoid the native window.confirm dialog, which blocks
  // the whole WebView and cannot be styled or translated consistently.
  const confirm = useCallback((options: ConfirmOptions) => new Promise<boolean>((resolve) => {
    // A second request supersedes the first instead of leaving it pending.
    confirmResolver.current?.(false);
    confirmResolver.current = resolve;
    setConfirmOptions(options);
  }), []);

  const resolveConfirm = useCallback((value: boolean) => {
    const resolve = confirmResolver.current;
    confirmResolver.current = null;
    setConfirmOptions(null);
    resolve?.(value);
  }, []);

  const applyInstances = useCallback((instances: InstanceView[]) => {
    setBootstrap((current) => current ? {
      ...current,
      instances,
      running: instances.filter((instance) => instance.running).map((instance) => instance.id),
    } : current);
    setSelectedId((current) => {
      if (current && instances.some((instance) => instance.id === current)) return current;
      return instances[0]?.id ?? null;
    });
  }, []);

  const refreshInstances = useCallback(async () => {
    try {
      const instances = await api.getInstances();
      applyInstances(instances);
    } catch (error) {
      pushToast('error', t('app.refreshFailed'), (error as Error).message);
    }
  }, [applyInstances, pushToast, t]);

  const reloadBootstrap = useCallback(async () => {
    const fresh = await api.bootstrap();
    setBootstrap(fresh);
    setBootError(fresh.initError);
    setSelectedId((current) => {
      if (current && fresh.instances.some((instance) => instance.id === current)) return current;
      const stored = window.localStorage.getItem(SELECTED_KEY);
      if (stored && fresh.instances.some((instance) => instance.id === stored)) return stored;
      return fresh.instances[0]?.id ?? null;
    });
  }, []);

  useEffect(() => {
    let alive = true;
    setLoading(true);
    api.bootstrap()
      .then((data) => {
        if (!alive) return;
        setBootstrap(data);
        setBootError(data.initError);
        const stored = window.localStorage.getItem(SELECTED_KEY);
        const next = stored && data.instances.some((instance) => instance.id === stored)
          ? stored
          : data.instances[0]?.id ?? null;
        setSelectedId(next);
        setLanguage(resolveStartupLanguage(data.settings.languageAuto, data.settings.language));
      })
      .catch((error: Error) => {
        if (alive) setBootError(error.message);
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => { alive = false; };
  }, [setLanguage]);

  useEffect(() => {
    if (!bootstrap || bootError || elevationBlocked) return;
    let alive = true;
    api.getLanSnapshot()
      .then((snapshot) => {
        if (alive) setLanPeerCount(snapshot.peers.length);
      })
      .catch(() => {
        // Discovery may still be starting; the lan:changed event will catch up.
      });
    return () => { alive = false; };
  }, [Boolean(bootstrap), Boolean(bootError), elevationBlocked]);

  useEffect(() => {
    if (selectedId) {
      try {
        window.localStorage.setItem(SELECTED_KEY, selectedId);
      } catch {
        // Storage is optional.
      }
    }
  }, [selectedId]);

  const saveSettings = useCallback(async (next: Settings) => {
    const saved = await api.saveSettings(next);
    setBootstrap((current) => current ? { ...current, settings: saved } : current);
    return saved;
  }, []);

  const selectInstance = useCallback((id: string) => setSelectedId(id), []);

  const openDataFolder = useCallback(async () => {
    try {
      await api.openDataFolder();
    } catch (error) {
      pushToast('error', t('app.openGameFolderFailed'), (error as Error).message);
    }
  }, [pushToast, t]);

  const openInstanceFolder = useCallback(async (id: string) => {
    try {
      await api.openInstanceFolder(id);
    } catch (error) {
      pushToast('error', t('app.openVersionFolderFailed'), (error as Error).message);
    }
  }, [pushToast, t]);

  const openConsole = useCallback((id: string) => {
    setConsoleId(id);
  }, []);

  const launch = useCallback(async (id: string) => {
    setLaunchingId(id);
    setLaunchPhase('preparing');
    try {
      await api.launchInstance(id, language);
    } catch (error) {
      setLaunchingId(null);
      pushToast('error', t('app.launchFailed'), (error as Error).message, {
        label: t('app.viewLogs'),
        onClick: () => openConsole(id),
      });
    }
  }, [pushToast, t, language, openConsole]);

  const launchToServer = useCallback(async (id: string, address: string) => {
    setLaunchingId(id);
    setLaunchPhase('preparing');
    try {
      await api.launchInstanceToServer(id, language, address);
    } catch (error) {
      setLaunchingId(null);
      pushToast('error', t('app.launchFailed'), (error as Error).message, {
        label: t('app.viewLogs'),
        onClick: () => openConsole(id),
      });
    }
  }, [pushToast, t, language, openConsole]);

  const stop = useCallback(async (id: string) => {
    try {
      await api.stopInstance(id);
    } catch (error) {
      pushToast('error', t('app.stopFailed'), (error as Error).message);
    }
  }, [pushToast, t]);

  const navigate = useCallback((next: PageKey) => {
    setPage(next);
    window.setTimeout(() => {
      document.querySelector<HTMLElement>('.page-scroll')?.scrollTo({ top: 0, behavior: 'smooth' });
    }, 30);
  }, []);

  useEffect(() => {
    if (bootstrap?.license?.state !== 'trial') return;
    const timer = window.setInterval(() => {
      api.getLicenseStatus()
        .then((next) => setBootstrap((current) => current ? { ...current, license: next } : current))
        .catch(() => {});
    }, 60_000);
    return () => window.clearInterval(timer);
  }, [bootstrap?.license?.state]);

  useEffect(() => {
    if (!bootstrap || bootError) return;
    return onEvent<LicenseStatus>('license:changed', (next) => {
      setBootstrap((current) => current ? { ...current, license: next } : current);
    });
  }, [Boolean(bootstrap), Boolean(bootError)]);

  useEffect(() => {
    if (!bootstrap || bootError) return () => {};
    const disposers = [
      onEvent<null>('instances:changed', () => { void refreshInstances(); }),
      onEvent<AccountState>('accounts:changed', (state) => {
        setBootstrap((current) => current ? {
          ...current,
          accounts: state.accounts,
          selectedAccountId: state.selectedId,
        } : current);
      }),
      onEvent<AccountLoginEvent>('auth:login', (event) => {
        if (event.success) {
          void reloadBootstrap().catch((error: Error) => {
            pushToast('error', t('app.refreshFailed'), error.message);
          });
          pushToast('success', t('app.msLoginSuccess'), event.account?.name);
        } else {
          pushToast('error', t('app.msLoginFailed'), event.message);
        }
      }),
      onEvent<{ instanceId: string }>('launch:window', (payload) => {
        setLaunchingId((current) => current === payload.instanceId ? null : current);
      }),
      onEvent<LanSnapshot>('lan:changed', (snapshot) => {
        setLanPeerCount(snapshot.peers.length);
      }),
      onEvent<null>('app:close-request', () => setExitOpen(true)),
      onEvent<LaunchState>('launch:state', (state) => {
        void refreshInstances();
        if (state.running) {
          setLaunchPhase('waiting');
          if (settingsRef.current?.showGameConsole !== false) {
            void openConsole(state.instanceId);
          }
          const instance = instancesRef.current.find((item) => item.id === state.instanceId);
          pushToast('success', t('app.gameStarted'), instance ? instance.name : undefined);
        } else {
          setLaunchingId((current) => current === state.instanceId ? null : current);
          if (state.stoppedByUser) {
            pushToast('info', t('app.gameStopped'));
          } else if (state.exitCode === 0) {
            pushToast('info', t('app.gameExited'));
          } else {
            pushToast('error', t('app.gameCrashed'), t('app.exitCode', { code: state.exitCode }), {
              label: t('app.viewLogs'),
              onClick: () => openConsole(state.instanceId),
            });
          }
        }
      }),
    ];
    return () => disposers.forEach((dispose) => dispose());
  }, [Boolean(bootstrap), Boolean(bootError), elevationBlocked, refreshInstances, openConsole, pushToast, reloadBootstrap, t]);

  useEffect(() => {
    const timers = toastTimers.current;
    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
    };
  }, []);

  useEffect(() => {
    if (!launchingId) return;
    const timer = window.setTimeout(() => setLaunchingId(null), 180_000);
    return () => window.clearTimeout(timer);
  }, [launchingId]);

  const instances = bootstrap?.instances ?? [];
  const settings = bootstrap?.settings ?? null;
  const accounts = bootstrap?.accounts ?? [];
  const selectedAccount = accounts.find((account) => account.id === bootstrap?.selectedAccountId) ?? accounts[0] ?? null;
  const selectedInstance = instances.find((instance) => instance.id === selectedId) ?? null;
  const activeConsole = instances.find((instance) => instance.id === consoleId) ?? null;
  const launchingInstance = instances.find((instance) => instance.id === launchingId) ?? null;

  const hasFeature = useCallback((feature: string) => {
    if (!licenseActive) return false;
    const normalized = feature.toLowerCase();
    return (bootstrap?.license?.features ?? []).some((granted) => {
      const value = granted.toLowerCase();
      return value === normalized || ['*', 'all', 'pro', 'commercial'].includes(value);
    });
  }, [bootstrap?.license?.features, licenseActive]);

  const openLicense = useCallback(() => setLicenseOpen(true), []);
  const closeLicense = useCallback(() => setLicenseOpen(false), []);

  const contextValue = useMemo(() => ({
    bootstrap,
    instances,
    settings,
    accounts,
    selectedAccount,
    page,
    theme,
    setTheme,
    language,
    locale,
    t,
    setLanguage,
    navigate,
    selectedId,
    selectedInstance,
    launchingId,
    selectInstance,
    refreshInstances,
    reloadBootstrap,
    saveSettings,
    openAccounts: () => setAccountsOpen(true),
    openCreate: () => setCreateOpen(true),
    openEdit: (id: string) => setEditId(id),
    askDelete: (id: string) => setDeleteId(id),
    openDataFolder,
    openInstanceFolder,
    launch,
    launchToServer,
    stop,
    openConsole: (id: string) => { void openConsole(id); },
    pushToast,
    confirm,
    lanPeerCount,
    isDemo,
    licenseOpen,
    openLicense,
    closeLicense,
    hasFeature,
  }), [
    bootstrap, instances, settings, accounts, selectedAccount, page, theme, setTheme,
    language, locale, t, setLanguage,
    navigate, selectedId, selectedInstance, launchingId, selectInstance, refreshInstances,
    reloadBootstrap, saveSettings, openDataFolder, openInstanceFolder, launch, launchToServer, stop, lanPeerCount,
    openConsole, pushToast, confirm, licenseOpen, openLicense, closeLicense, hasFeature,
  ]);

  if (loading) {
    return (
      <div className="boot-screen">
        <div className="boot-glow" />
        <LogoMark size={72} />
        <div className="boot-title">MYGO</div>
        <div className="boot-sub">{t('app.bootSub')}</div>
        <FunLine language={language} className="boot-fun" />
        <div className="boot-bar"><span /></div>
      </div>
    );
  }

  if (bootError || !bootstrap) {
    return (
      <div className="boot-screen boot-error">
        <div className="boot-error-icon"><TriangleAlert size={28} /></div>
        <h1>{t('app.initFailed')}</h1>
        <p>{bootError || t('app.readConfigFailed')}</p>
        <button
          type="button"
          className="btn btn-primary"
          onClick={() => {
            setLoading(true);
            // reloadBootstrap already sets bootError from initError.
            reloadBootstrap()
              .catch((error: Error) => setBootError(error.message))
              .finally(() => setLoading(false));
          }}
        >
          <RefreshCw size={15} /> {t('common.retry')}
        </button>
        <span className="boot-legal">MyGoLuncher · v{bootstrap?.version ?? '1.0.3'}</span>
      </div>
    );
  }

  const pages: Record<PageKey, ReactNode> = {
    home: <HomePage />,
    versions: <VersionsPage />,
    instanceSettings: <InstanceSettingsPage />,
    resources: <ResourcesPage />,
    modpacks: <ModpacksPage />,
    servers: hasFeature('server') ? <ServersPage /> : <PaidFeaturePage feature="server" />,
    lan: hasFeature('lan') ? <LanPage /> : <PaidFeaturePage feature="lan" />,
    settings: <SettingsPage />,
    about: <AboutPage />,
  };

  return (
    <AppContext.Provider value={contextValue}>
      {elevationBlocked ? (
        <ElevationGate onConfirmed={reloadBootstrap} />
      ) : (
        <>
      <div className="app-shell">
        <Sidebar />
        <div className="app-main">
          <TopBar />
          <main className="page-scroll">
            <div className="page-stage" key={page}>
              {pages[page]}
            </div>
          </main>
        </div>
      </div>

      <AccountModal open={accountsOpen} onClose={() => setAccountsOpen(false)} />
      <CreateInstanceModal open={createOpen} onClose={() => setCreateOpen(false)} />
      <EditInstanceModal id={editId} open={Boolean(editId)} onClose={() => setEditId(null)} />
      <ConfirmDeleteModal id={deleteId} open={Boolean(deleteId)} onClose={() => setDeleteId(null)} />

      {activeConsole && (
        <ConsoleDrawer
          instance={activeConsole}
          onClose={() => setConsoleId(null)}
          onStop={() => void stop(activeConsole.id)}
          onOpenFolder={() => void openInstanceFolder(activeConsole.id)}
        />
      )}

      {launchingInstance && (
        <LaunchOverlay
          instance={launchingInstance}
          phase={launchPhase}
          onDismiss={() => setLaunchingId(null)}
        />
      )}

      <TaskTray />
      <ToastStack toasts={toasts} onDismiss={dismissToast} withConsole={Boolean(activeConsole)} />
      <ConfirmDialog options={confirmOptions} onResolve={resolveConfirm} />

      {licenseOpen && (
        <LicenseGate
          status={bootstrap.license ?? {
            state: 'error', configured: false, activated: false, machineCode: '', features: [],
            message: t('license.failed'), graceDays: 7,
          }}
          onActivated={reloadBootstrap}
          onClose={closeLicense}
        />
      )}
        </>
      )}

      <ExitConfirmModal open={exitOpen} onClose={() => setExitOpen(false)} />
    </AppContext.Provider>
  );
}
