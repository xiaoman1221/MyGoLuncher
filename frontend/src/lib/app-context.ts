import { createContext, useContext } from 'react';
import type {
  AccountState,
  AccountView,
  Bootstrap,
  InstanceView,
  PageKey,
  Settings,
  TaskProgress,
  Theme,
} from './types';

export type ToastKind = 'success' | 'error' | 'info';

export interface Toast {
  id: number;
  kind: ToastKind;
  title: string;
  message?: string;
}

export interface AppContextValue {
  bootstrap: Bootstrap | null;
  instances: InstanceView[];
  settings: Settings | null;
  accounts: AccountView[];
  selectedAccount: AccountView | null;
  page: PageKey;
  theme: Theme;
  setTheme: (theme: Theme) => void;
  navigate: (page: PageKey) => void;
  selectedId: string | null;
  selectedInstance: InstanceView | null;
  selectInstance: (id: string) => void;
  refreshInstances: () => Promise<void>;
  reloadBootstrap: () => Promise<void>;
  saveSettings: (settings: Settings) => Promise<void>;
  openAccounts: () => void;
  openCreate: () => void;
  openEdit: (id: string) => void;
  askDelete: (id: string) => void;
  openDataFolder: () => Promise<void>;
  openInstanceFolder: (id: string) => Promise<void>;
  launch: (id: string) => Promise<void>;
  stop: (id: string) => Promise<void>;
  openConsole: (id: string) => void;
  pushToast: (kind: ToastKind, title: string, message?: string) => void;
  tasks: TaskProgress[];
  isDemo: boolean;
}

export const AppContext = createContext<AppContextValue | null>(null);

export function useApp(): AppContextValue {
  const value = useContext(AppContext);
  if (!value) throw new Error('useApp must be used inside AppContext.Provider');
  return value;
}
