import { useEffect, useState } from 'react';
import {
  BadgeCheck,
  Check,
  Copy,
  ExternalLink,
  Globe2,
  KeyRound,
  LoaderCircle,
  LogIn,
  Pencil,
  Server,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  UserRound,
} from 'lucide-react';
import { api, isDemo } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import type { AccountLoginEvent, AccountView, AuthlibServer, DeviceCodeView, YSSDeviceView } from '../lib/types';
import { accountKindKey } from '../lib/i18n';
import { cn, initials } from '../lib/format';
import { Modal } from './Modal';

type LoginTab = 'microsoft' | 'authlib' | 'offline';

const tabs: Array<{ key: LoginTab; labelKey: 'account.tab.microsoft' | 'account.tab.authlib' | 'account.tab.offline'; icon: typeof BadgeCheck }> = [
  { key: 'microsoft', labelKey: 'account.tab.microsoft', icon: BadgeCheck },
  { key: 'authlib', labelKey: 'account.tab.authlib', icon: Globe2 },
  { key: 'offline', labelKey: 'account.tab.offline', icon: UserRound },
];

const fallbackServers: AuthlibServer[] = [
  { id: 'littleskin', name: 'LittleSkin', url: 'https://littleskin.cn/api/yggdrasil' },
  { id: 'yss', name: 'YggdrasilSkinServer', url: 'https://yss.1v.fit' },
];

function kindClass(account: AccountView) {
  if (account.kind === 'microsoft') return 'kind-microsoft';
  if (account.kind === 'authlib') return 'kind-authlib';
  return 'kind-offline';
}

function hostOf(raw: string): string {
  try {
    return new URL(raw.includes('://') ? raw : `https://${raw}`).hostname.toLowerCase();
  } catch {
    return '';
  }
}

function isYSSServer(raw: string): boolean {
  const host = hostOf(raw);
  return host === 'yss.1v.fit' || host.endsWith('.yss.1v.fit');
}

function canOpenVerification(url: string, server: string): boolean {
  if (!url) return false;
  try {
    const target = new URL(url);
    if (target.protocol !== 'http:' && target.protocol !== 'https:') return false;
    const expected = hostOf(server);
    return !expected || target.hostname.toLowerCase() === expected;
  } catch {
    return false;
  }
}

export function AccountModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const {
    accounts, selectedAccount, settings, reloadBootstrap, pushToast, t,
  } = useApp();
  const [tab, setTab] = useState<LoginTab>('microsoft');
  const [device, setDevice] = useState<DeviceCodeView | null>(null);
  const [msBusy, setMsBusy] = useState(false);
  const [msError, setMsError] = useState('');
  const [msDone, setMsDone] = useState(false);
  const [copied, setCopied] = useState(false);
  const [offlineName, setOfflineName] = useState('');
  const [editingOfflineId, setEditingOfflineId] = useState<string | null>(null);
  const [servers, setServers] = useState<AuthlibServer[]>(fallbackServers);
  const [authServer, setAuthServer] = useState(fallbackServers[0].url);
  const [authUser, setAuthUser] = useState('');
  const [authPassword, setAuthPassword] = useState('');
  const [authBusy, setAuthBusy] = useState(false);
  const [authError, setAuthError] = useState('');
  const [offlineBusy, setOfflineBusy] = useState(false);
  const [removing, setRemoving] = useState<string | null>(null);
  const [yssDevice, setYssDevice] = useState<YSSDeviceView | null>(null);
  const [yssBusy, setYssBusy] = useState(false);
  const [yssError, setYssError] = useState('');
  const [yssDone, setYssDone] = useState(false);
  const [yssCopied, setYssCopied] = useState(false);

  useEffect(() => {
    if (!open) return;
    setMsError('');
    setMsDone(false);
    setAuthError('');
    setYssDevice(null);
    setYssBusy(false);
    setYssError('');
    setYssDone(false);
    const offline = accounts.find((account) => account.kind === 'offline');
    setOfflineName(offline?.name ?? settings?.playerName ?? 'Steve');
    setEditingOfflineId(null);
    setTab('microsoft');
    void api.authlibServers()
      .then((list) => {
        if (list.length === 0) return;
        setServers(list);
        setAuthServer((current) => current || list[0].url);
      })
      .catch(() => {
        // Built-in presets remain available if the backend call fails.
      });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  useEffect(() => {
    if (!open) return;
    return onEvent<AccountLoginEvent>('auth:login', (event) => {
      setMsBusy(false);
      if (event.success) {
        setDevice(null);
        setMsDone(true);
        setMsError('');
        void reloadBootstrap();
      } else {
        setMsError(event.message);
      }
    });
  }, [open, reloadBootstrap]);

  useEffect(() => {
    if (!open) return;
    return onEvent<AccountLoginEvent>('auth:yss', (event) => {
      setYssBusy(false);
      setYssDevice(null);
      if (event.success) {
        setYssDone(true);
        setYssError('');
        void reloadBootstrap();
      } else {
        setYssError(event.message);
      }
    });
  }, [open, reloadBootstrap]);

  const close = () => {
    if (msBusy) void api.cancelMicrosoftLogin();
    if (yssBusy) void api.cancelYSSLogin();
    setDevice(null);
    setMsBusy(false);
    setMsError('');
    setYssDevice(null);
    setYssBusy(false);
    onClose();
  };

  const startMicrosoft = async () => {
    setMsBusy(true);
    setMsError('');
    setMsDone(false);
    setDevice(null);
    try {
      const code = await api.beginMicrosoftLogin();
      setDevice(code);
      if (code.verificationUri) void api.openURL(code.verificationUri);
    } catch (error) {
      setMsBusy(false);
      setMsError((error as Error).message);
    }
  };

  const cancelMicrosoft = async () => {
    try {
      await api.cancelMicrosoftLogin();
    } finally {
      setMsBusy(false);
      setDevice(null);
    }
  };

  const copyCode = async () => {
    if (!device) return;
    try {
      await navigator.clipboard.writeText(device.userCode);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  const startYSS = async () => {
    setYssBusy(true);
    setYssError('');
    setYssDone(false);
    setYssDevice(null);
    try {
      const code = await api.beginYSSLogin(authServer);
      setYssDevice(code);
      const target = code.verificationUriComplete || code.verificationUri;
      if (canOpenVerification(target, authServer)) {
        void api.openURL(target);
      } else if (target) {
        setYssError(t('account.yss.openFailed'));
      }
    } catch (error) {
      setYssBusy(false);
      setYssError((error as Error).message);
    }
  };

  const cancelYSS = async () => {
    try {
      await api.cancelYSSLogin();
    } finally {
      setYssBusy(false);
      setYssDevice(null);
    }
  };

  const copyYSS = async () => {
    if (!yssDevice) return;
    try {
      await navigator.clipboard.writeText(yssDevice.userCode);
      setYssCopied(true);
      window.setTimeout(() => setYssCopied(false), 1500);
    } catch {
      setYssCopied(false);
    }
  };

  const selectAccount = async (account: AccountView) => {
    try {
      await api.selectAccount(account.id);
      await reloadBootstrap();
      pushToast('success', t('account.switched'), account.name);
    } catch (error) {
      pushToast('error', t('account.switchFailed'), (error as Error).message);
    }
  };

  const removeAccount = async (account: AccountView) => {
    setRemoving(account.id);
    try {
      await api.removeAccount(account.id);
      await reloadBootstrap();
      pushToast('success', t('account.removed'));
    } catch (error) {
      pushToast('error', t('account.removeFailed'), (error as Error).message);
    } finally {
      setRemoving(null);
    }
  };

  const loginAuthlib = async () => {
    setAuthBusy(true);
    setAuthError('');
    try {
      await api.loginAuthlib(authServer, authUser, authPassword);
      await reloadBootstrap();
      setAuthPassword('');
      pushToast('success', t('account.authlibSuccess'), authUser);
    } catch (error) {
      setAuthError((error as Error).message);
    } finally {
      setAuthBusy(false);
    }
  };

  const submitOffline = async () => {
    const name = offlineName.trim();
    if (!name) {
      pushToast('error', t('account.nameRequired'));
      return;
    }
    setOfflineBusy(true);
    try {
      if (editingOfflineId) {
        await api.renameOfflineAccount(editingOfflineId, name);
        pushToast('success', t('account.offlineSaved'), name);
      } else {
        await api.addOfflineAccount(name);
        pushToast('success', t('account.offlineAdded'), name);
      }
      await reloadBootstrap();
      setEditingOfflineId(null);
    } catch (error) {
      pushToast('error', t('account.offlineSaveFailed'), (error as Error).message);
    } finally {
      setOfflineBusy(false);
    }
  };

  const editOffline = (account: AccountView) => {
    setEditingOfflineId(account.id);
    setOfflineName(account.name);
    setTab('offline');
  };

  return (
    <Modal
      open={open}
      onClose={close}
      title={t('account.title')}
      eyebrow="ACCOUNTS"
      description={t('account.description')}
      size="xl"
    >
      <div className="account-layout">
        <aside className="account-side">
          <div className="account-side-head">
            <span>{t('account.saved')}</span>
            <span className="account-count">{accounts.length}</span>
          </div>
          <div className="account-list">
            {accounts.map((account) => {
              const active = account.id === (selectedAccount?.id ?? '');
              return (
                <div className={cn('account-row', active && 'is-active', kindClass(account))} key={account.id}>
                  <div className="account-avatar">{initials(account.name)}</div>
                  <div className="account-row-copy">
                    <div className="account-row-title">
                      <strong>{account.name}</strong>
                      {active && <span className="account-active"><Check size={10} /> {t('account.inUse')}</span>}
                    </div>
                    <span>{t(accountKindKey(account.kind))}{account.serverUrl ? ` · ${account.serverUrl.replace(/^https?:\/\//, '')}` : ''}</span>
                  </div>
                  <div className="account-row-actions">
                    {!active && (
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => void selectAccount(account)}>
                        {t('account.switch')}
                      </button>
                    )}
                    {account.kind === 'offline' && (
                      <button type="button" className="icon-button icon-button-sm" title={t('account.editOffline')} onClick={() => editOffline(account)}>
                        <Pencil size={14} />
                      </button>
                    )}
                    <button
                      type="button"
                      className="icon-button icon-button-sm account-remove"
                      title={accounts.length <= 1 ? t('account.keepOne') : t('account.remove')}
                      onClick={() => void removeAccount(account)}
                      disabled={removing === account.id || accounts.length <= 1}
                    >
                      {removing === account.id ? <LoaderCircle size={14} className="spin" /> : <Trash2 size={14} />}
                    </button>
                  </div>
                </div>
              );
            })}
          </div>
        </aside>

        <section className="account-main">
          <div className="account-tabs">
            {tabs.map((item) => {
              const Icon = item.icon;
              return (
                <button
                  key={item.key}
                  type="button"
                  className={cn('account-tab', tab === item.key && 'is-active')}
                  onClick={() => setTab(item.key)}
                >
                  <Icon size={15} />
                  {t(item.labelKey)}
                </button>
              );
            })}
          </div>

          {tab === 'microsoft' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon microsoft"><BadgeCheck size={22} /></div>
                <div>
                  <h3>{t('account.microsoft.title')}</h3>
                  <p>{t('account.microsoft.body')}</p>
                </div>
              </div>

              {!device && !msDone && (
                <button type="button" className="btn btn-primary btn-lg full-width" onClick={() => void startMicrosoft()} disabled={msBusy}>
                  {msBusy ? <LoaderCircle size={17} className="spin" /> : <LogIn size={17} />}
                  {msBusy ? t('account.microsoft.requesting') : t('account.microsoft.login')}
                </button>
              )}

              {device && (
                <div className="device-code-card">
                  <p>{t('account.microsoft.deviceHint')}</p>
                  <button type="button" className="device-code" onClick={() => void copyCode()}>
                    <strong>{device.userCode}</strong>
                    {copied ? <Check size={16} /> : <Copy size={16} />}
                  </button>
                  <a className="device-link" href={device.verificationUri} onClick={(event) => { event.preventDefault(); void api.openURL(device.verificationUri); }}>
                    {device.verificationUri} <ExternalLink size={13} />
                  </a>
                  <div className="device-status">
                    <LoaderCircle size={14} className="spin" />
                    <span>{t('account.microsoft.waiting')}</span>
                  </div>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void cancelMicrosoft()}>{t('common.cancel')}</button>
                </div>
              )}

              {msDone && (
                <div className="account-success">
                  <BadgeCheck size={18} />
                  <span>{t('account.microsoft.success')}</span>
                </div>
              )}
              {msError && (
                <div className="inline-banner banner-error">
                  <TriangleAlert size={16} />
                  <div><strong>{t('account.microsoft.failed')}</strong><span>{msError}</span></div>
                </div>
              )}
              {isDemo && !device && (
                <p className="account-note">{t('account.microsoft.demoNote')}</p>
              )}
            </div>
          )}

          {tab === 'authlib' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon authlib"><Globe2 size={22} /></div>
                <div>
                  <h3>{t('account.authlib.title')}</h3>
                  <p>{t('account.authlib.body')}</p>
                </div>
              </div>
              <div className="authlib-presets">
                {servers.map((server) => (
                  <button
                    type="button"
                    key={server.id}
                    className={cn('authlib-preset', authServer === server.url && 'is-active')}
                    onClick={() => setAuthServer(server.url)}
                  >
                    <Globe2 size={15} />
                    <span>
                      <strong>{server.name}</strong>
                      <small>{server.url}</small>
                    </span>
                  </button>
                ))}
              </div>
              {isYSSServer(authServer) && (
                <div className="yss-auth-block">
                  <button
                    type="button"
                    className="btn btn-primary full-width"
                    onClick={() => void startYSS()}
                    disabled={yssBusy}
                  >
                    {yssBusy ? <LoaderCircle size={16} className="spin" /> : <ShieldCheck size={16} />}
                    {yssBusy ? t('account.yss.waiting') : t('account.yss.oneClick')}
                  </button>
                  <p className="account-note">{t('account.yss.hint')}</p>

                  {yssDevice && (
                    <div className="device-code-card">
                      <p>{t('account.yss.codeHint')}</p>
                      <button type="button" className="device-code" onClick={() => void copyYSS()}>
                        <strong>{yssDevice.userCode}</strong>
                        {yssCopied ? <Check size={16} /> : <Copy size={16} />}
                      </button>
                      <a
                        className="device-link"
                        href={yssDevice.verificationUriComplete || yssDevice.verificationUri}
                        onClick={(event) => {
                          event.preventDefault();
                          const target = yssDevice.verificationUriComplete || yssDevice.verificationUri;
                          if (canOpenVerification(target, authServer)) {
                            void api.openURL(target);
                          } else {
                            setYssError(t('account.yss.openFailed'));
                          }
                        }}
                      >
                        {yssDevice.verificationUriComplete || yssDevice.verificationUri} <ExternalLink size={13} />
                      </a>
                      <div className="device-status">
                        <LoaderCircle size={14} className="spin" />
                        <span>{t('account.yss.waiting')}</span>
                      </div>
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => void cancelYSS()}>
                        {t('account.yss.cancel')}
                      </button>
                    </div>
                  )}

                  {yssDone && (
                    <div className="account-success">
                      <ShieldCheck size={18} />
                      <span>{t('account.yss.success')}</span>
                    </div>
                  )}
                  {yssError && (
                    <div className="inline-banner banner-error">
                      <TriangleAlert size={16} />
                      <div><strong>{t('account.yss.failed')}</strong><span>{yssError}</span></div>
                    </div>
                  )}
                </div>
              )}
              <label className="field">
                <span className="field-label"><Server size={14} /> {t('account.authlib.server')}</span>
                <input
                  className="text-input mono"
                  value={authServer}
                  onChange={(event) => setAuthServer(event.target.value)}
                  placeholder="https://littleskin.cn/api/yggdrasil"
                />
              </label>
              <div className="settings-grid-2">
                <label className="field">
                  <span className="field-label"><UserRound size={14} /> {t('account.authlib.username')}</span>
                  <input className="text-input" value={authUser} onChange={(event) => setAuthUser(event.target.value)} autoComplete="username" />
                </label>
                <label className="field">
                  <span className="field-label"><KeyRound size={14} /> {t('account.authlib.password')}</span>
                  <input
                    className="text-input"
                    type="password"
                    value={authPassword}
                    onChange={(event) => setAuthPassword(event.target.value)}
                    autoComplete="current-password"
                    onKeyDown={(event) => { if (event.key === 'Enter') void loginAuthlib(); }}
                  />
                </label>
              </div>
              {authError && (
                <div className="inline-banner banner-error">
                  <TriangleAlert size={16} />
                  <div><strong>{t('account.authlib.failed')}</strong><span>{authError}</span></div>
                </div>
              )}
              <button type="button" className="btn btn-primary" onClick={() => void loginAuthlib()} disabled={authBusy || !authUser || !authPassword}>
                {authBusy ? <LoaderCircle size={16} className="spin" /> : <LogIn size={16} />}
                {authBusy ? t('account.authlib.verifying') : t('account.authlib.login')}
              </button>
              <p className="account-note">{t('account.authlib.note')}</p>
            </div>
          )}

          {tab === 'offline' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon offline"><UserRound size={22} /></div>
                <div>
                  <h3>{editingOfflineId ? t('account.offline.renameTitle') : t('account.offline.addTitle')}</h3>
                  <p>{editingOfflineId ? t('account.offline.renameBody') : t('account.offline.addBody')}</p>
                </div>
              </div>
              <label className="field">
                <span className="field-label">{t('account.offline.name')} <small>{t('account.offline.nameHint')}</small></span>
                <input
                  className="text-input"
                  value={offlineName}
                  onChange={(event) => setOfflineName(event.target.value)}
                  maxLength={16}
                  onKeyDown={(event) => { if (event.key === 'Enter') void submitOffline(); }}
                  autoFocus
                />
              </label>
              <div className="button-row">
                <button type="button" className="btn btn-primary" onClick={() => void submitOffline()} disabled={offlineBusy}>
                  {offlineBusy ? <LoaderCircle size={16} className="spin" /> : <UserRound size={16} />}
                  {editingOfflineId ? t('account.offline.saveName') : t('account.offline.add')}
                </button>
                {editingOfflineId && (
                  <button type="button" className="btn btn-ghost" onClick={() => { setEditingOfflineId(null); setOfflineName(''); }}>
                    {t('account.offline.new')}
                  </button>
                )}
              </div>
            </div>
          )}
        </section>
      </div>
    </Modal>
  );
}
