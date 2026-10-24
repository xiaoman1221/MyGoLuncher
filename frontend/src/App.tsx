import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { RefreshCw, TriangleAlert } from 'lucide-react';
import { api, isDemo } from './lib/api';
import { onEvent } from './lib/events';
import { AppContext, type Toast, type ToastKind } from './lib/app-context';
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
  LaunchState,
  PageKey,
  Settings,
  TaskProgress,
  Theme,
} from './lib/types';
import { Sidebar } from './components/Sidebar';
import { TopBar } from './components/TopBar';
import { TaskTray } from './components/TaskTray';
import { ToastStack } from './components/ToastStack';
import { ConsoleDrawer } from './components/ConsoleDrawer';
import { LogoMark } from './components/LogoMark';
import { CreateInstanceModal } from './components/CreateInstanceModal';
import { EditInstanceModal } from './components/EditInstanceModal';
import { ConfirmDeleteModal } from './components/ConfirmDeleteModal';
import { AccountModal } from './components/AccountModal';
import { LaunchOverlay } from './components/LaunchOverlay';
import { HomePage } from './pages/HomePage';
import { VersionsPage } from './pages/VersionsPage';
import { InstanceSettingsPage } from './pages/InstanceSettingsPage';
import { ResourcesPage } from './pages/ResourcesPage';
import { ModpacksPage } from './pages/ModpacksPage';
import { SettingsPage } from './pages/SettingsPage';

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
  const [tasks, setTasks] = useState<Record<string, TaskProgress>>({});
  const [toasts, setToasts] = useState<Toast[]>([]);
  const [consoleId, setConsoleId] = useState<string | null>(null);
  const [consoleLogs, setConsoleLogs] = useState<Record<string, string[]>>({});
  const [createOpen, setCreateOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [deleteId, setDeleteId] = useState<string | null>(null);
  const [accountsOpen, setAccountsOpen] = useState(false);
  const [launchingId, setLaunchingId] = useState<string | null>(null);
  const [launchPhase, setLaunchPhase] = useState<'preparing' | 'waiting'>('preparing');

  const toastId = useRef(0);
  const taskTimers = useRef(new Map<string, number>());
  const toastTimers = useRef(new Map<number, number>());
  const settingsRef = useRef<Settings | null>(null);

  useEffect(() => {
    settingsRef.current = bootstrap?.settings ?? null;
  }, [bootstrap?.settings]);

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

  const pushToast = useCallback((kind: ToastKind, title: string, message?: string) => {
    const id = ++toastId.current;
    setToasts((current) => [...current.slice(-4), { id, kind, title, message }]);
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

  const launch = useCallback(async (id: string) => {
    setLaunchingId(id);
    setLaunchPhase('preparing');
    try {
      await api.launchInstance(id, language);
    } catch (error) {
      setLaunchingId(null);
      pushToast('error', t('app.launchFailed'), (error as Error).message);
    }
  }, [pushToast, t, language]);

  const stop = useCallback(async (id: string) => {
    try {
      await api.stopInstance(id);
    } catch (error) {
      pushToast('error', t('app.stopFailed'), (error as Error).message);
    }
  }, [pushToast, t]);

  const openConsole = useCallback(async (id: string) => {
    setConsoleId(id);
    try {
      const logs = await api.getInstanceLog(id);
      if (logs.length > 0) {
        setConsoleLogs((current) => ({ ...current, [id]: logs.slice(-600) }));
      }
    } catch {
      // Live events still populate the console.
    }
  }, []);

  const navigate = useCallback((next: PageKey) => {
    setPage(next);
    window.setTimeout(() => {
      document.querySelector<HTMLElement>('.page-scroll')?.scrollTo({ top: 0, behavior: 'smooth' });
    }, 30);
  }, []);

  useEffect(() => {
    if (!bootstrap || bootError) return () => {};
    const disposers = [
      onEvent<TaskProgress>('task:progress', (task) => {
        setTasks((current) => ({ ...current, [task.taskId]: task }));
        const existing = taskTimers.current.get(task.taskId);
        if (existing) window.clearTimeout(existing);
        if (task.done) {
          const timer = window.setTimeout(() => {
            setTasks((current) => {
              const next = { ...current };
              delete next[task.taskId];
              return next;
            });
            taskTimers.current.delete(task.taskId);
          }, task.failed ? 8000 : 4200);
          taskTimers.current.set(task.taskId, timer);
        }
      }),
      onEvent<null>('instances:changed', () => { void refreshInstances(); }),
      onEvent<{ instanceId: string; line: string }>('launch:log', (payload) => {
        setConsoleLogs((current) => {
          const lines = current[payload.instanceId] ?? [];
          return { ...current, [payload.instanceId]: [...lines.slice(-599), payload.line] };
        });
      }),
      onEvent<AccountState>('accounts:changed', (state) => {
        setBootstrap((current) => current ? {
          ...current,
          accounts: state.accounts,
          selectedAccountId: state.selectedId,
        } : current);
      }),
      onEvent<AccountLoginEvent>('auth:login', (event) => {
        if (event.success) {
          void reloadBootstrap();
          pushToast('success', t('app.msLoginSuccess'), event.account?.name);
        } else {
          pushToast('error', t('app.msLoginFailed'), event.message);
        }
      }),
      onEvent<{ instanceId: string }>('launch:window', (payload) => {
        setLaunchingId((current) => current === payload.instanceId ? null : current);
      }),
      onEvent<LaunchState>('launch:state', (state) => {
        void refreshInstances();
        if (state.running) {
          setLaunchPhase('waiting');
          if (settingsRef.current?.showGameConsole !== false) {
            void openConsole(state.instanceId);
          }
          const instance = bootstrap.instances.find((item) => item.id === state.instanceId);
          pushToast('success', t('app.gameStarted'), instance ? instance.name : undefined);
        } else {
          setLaunchingId((current) => current === state.instanceId ? null : current);
          if (state.stoppedByUser) {
            pushToast('info', t('app.gameStopped'));
          } else {
            pushToast(
              state.exitCode === 0 ? 'info' : 'error',
              state.exitCode === 0 ? t('app.gameExited') : t('app.gameCrashed'),
              state.exitCode === 0 ? undefined : t('app.exitCode', { code: state.exitCode }),
            );
          }
        }
      }),
    ];
    return () => disposers.forEach((dispose) => dispose());
  }, [Boolean(bootstrap), Boolean(bootError), refreshInstances, openConsole, pushToast, reloadBootstrap, t]);

  useEffect(() => {
    const timers = taskTimers.current;
    const toastT = toastTimers.current;
    return () => {
      timers.forEach((timer) => window.clearTimeout(timer));
      toastT.forEach((timer) => window.clearTimeout(timer));
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
    stop,
    openConsole: (id: string) => { void openConsole(id); },
    pushToast,
    tasks: Object.values(tasks),
    isDemo,
  }), [
    bootstrap, instances, settings, accounts, selectedAccount, page, theme, setTheme,
    language, locale, t, setLanguage,
    navigate, selectedId, selectedInstance, launchingId, selectInstance, refreshInstances,
    reloadBootstrap, saveSettings, openDataFolder, openInstanceFolder, launch, stop,
    openConsole, pushToast, tasks,
  ]);

  if (loading) {
    return (
      <div className="boot-screen">
        <div className="boot-glow" />
        <LogoMark size={72} />
        <div className="boot-title">MYGO</div>
        <div className="boot-sub">{t('app.bootSub')}</div>
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
            reloadBootstrap()
              .then(() => setBootError(''))
              .catch((error: Error) => setBootError(error.message))
              .finally(() => setLoading(false));
          }}
        >
          <RefreshCw size={15} /> {t('common.retry')}
        </button>
        <span className="boot-legal">MyGoLuncher · v1.0.0</span>
      </div>
    );
  }

  const pages: Record<PageKey, ReactNode> = {
    home: <HomePage />,
    versions: <VersionsPage />,
    instanceSettings: <InstanceSettingsPage />,
    resources: <ResourcesPage />,
    modpacks: <ModpacksPage />,
    settings: <SettingsPage />,
  };

  return (
    <AppContext.Provider value={contextValue}>
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
          logs={consoleLogs[activeConsole.id] ?? []}
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

      <TaskTray tasks={Object.values(tasks)} />
      <ToastStack toasts={toasts} onDismiss={dismissToast} withConsole={Boolean(activeConsole)} />
    </AppContext.Provider>
  );
}
