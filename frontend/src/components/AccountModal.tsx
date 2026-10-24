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
  Trash2,
  TriangleAlert,
  UserRound,
} from 'lucide-react';
import { api, isDemo } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import type { AccountLoginEvent, AccountView, AuthlibServer, DeviceCodeView } from '../lib/types';
import { cn, initials } from '../lib/format';
import { Modal } from './Modal';

type LoginTab = 'microsoft' | 'authlib' | 'offline';

const tabs: Array<{ key: LoginTab; label: string; icon: typeof BadgeCheck }> = [
  { key: 'microsoft', label: 'Microsoft 正版', icon: BadgeCheck },
  { key: 'authlib', label: '外置登录', icon: Globe2 },
  { key: 'offline', label: '离线模式', icon: UserRound },
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

export function AccountModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const {
    accounts, selectedAccount, settings, reloadBootstrap, pushToast,
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

  useEffect(() => {
    if (!open) return;
    setMsError('');
    setMsDone(false);
    setAuthError('');
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

  const close = () => {
    if (msBusy) void api.cancelMicrosoftLogin();
    setDevice(null);
    setMsBusy(false);
    setMsError('');
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

  const selectAccount = async (account: AccountView) => {
    try {
      await api.selectAccount(account.id);
      await reloadBootstrap();
      pushToast('success', '已切换账户', account.name);
    } catch (error) {
      pushToast('error', '切换账户失败', (error as Error).message);
    }
  };

  const removeAccount = async (account: AccountView) => {
    setRemoving(account.id);
    try {
      await api.removeAccount(account.id);
      await reloadBootstrap();
      pushToast('success', '账户已移除');
    } catch (error) {
      pushToast('error', '移除账户失败', (error as Error).message);
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
      pushToast('success', '外置登录成功', authUser);
    } catch (error) {
      setAuthError((error as Error).message);
    } finally {
      setAuthBusy(false);
    }
  };

  const submitOffline = async () => {
    const name = offlineName.trim();
    if (!name) {
      pushToast('error', '请输入离线玩家名');
      return;
    }
    setOfflineBusy(true);
    try {
      if (editingOfflineId) {
        await api.renameOfflineAccount(editingOfflineId, name);
        pushToast('success', '离线账户名称已更新', name);
      } else {
        await api.addOfflineAccount(name);
        pushToast('success', '离线账户已添加', name);
      }
      await reloadBootstrap();
      setEditingOfflineId(null);
    } catch (error) {
      pushToast('error', '离线账户保存失败', (error as Error).message);
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
      title="账户"
      eyebrow="ACCOUNTS"
      description="选择启动游戏时使用的身份。账户数量不限。"
      size="xl"
    >
      <div className="account-layout">
        <aside className="account-side">
          <div className="account-side-head">
            <span>已保存账户</span>
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
                      {active && <span className="account-active"><Check size={10} /> 使用中</span>}
                    </div>
                    <span>{account.label}{account.serverUrl ? ` · ${account.serverUrl.replace(/^https?:\/\//, '')}` : ''}</span>
                  </div>
                  <div className="account-row-actions">
                    {!active && (
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => void selectAccount(account)}>
                        切换
                      </button>
                    )}
                    {account.kind === 'offline' && (
                      <button type="button" className="icon-button icon-button-sm" title="编辑离线账户" onClick={() => editOffline(account)}>
                        <Pencil size={14} />
                      </button>
                    )}
                    <button
                      type="button"
                      className="icon-button icon-button-sm account-remove"
                      title={accounts.length <= 1 ? '至少保留一个账户' : '移除账户'}
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
                  {item.label}
                </button>
              );
            })}
          </div>

          {tab === 'microsoft' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon microsoft"><BadgeCheck size={22} /></div>
                <div>
                  <h3>Microsoft 正版登录</h3>
                  <p>使用设备码在浏览器中完成授权，自动获取玩家名与 UUID。</p>
                </div>
              </div>

              {!device && !msDone && (
                <button type="button" className="btn btn-primary btn-lg full-width" onClick={() => void startMicrosoft()} disabled={msBusy}>
                  {msBusy ? <LoaderCircle size={17} className="spin" /> : <LogIn size={17} />}
                  {msBusy ? '正在获取登录代码…' : '使用 Microsoft 登录'}
                </button>
              )}

              {device && (
                <div className="device-code-card">
                  <p>在浏览器中打开链接并输入下面的代码。</p>
                  <button type="button" className="device-code" onClick={() => void copyCode()}>
                    <strong>{device.userCode}</strong>
                    {copied ? <Check size={16} /> : <Copy size={16} />}
                  </button>
                  <a className="device-link" href={device.verificationUri} onClick={(event) => { event.preventDefault(); void api.openURL(device.verificationUri); }}>
                    {device.verificationUri} <ExternalLink size={13} />
                  </a>
                  <div className="device-status">
                    <LoaderCircle size={14} className="spin" />
                    <span>等待授权完成…</span>
                  </div>
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void cancelMicrosoft()}>取消</button>
                </div>
              )}

              {msDone && (
                <div className="account-success">
                  <BadgeCheck size={18} />
                  <span>正版账户已添加。</span>
                </div>
              )}
              {msError && (
                <div className="inline-banner banner-error">
                  <TriangleAlert size={16} />
                  <div><strong>登录未完成</strong><span>{msError}</span></div>
                </div>
              )}
              {isDemo && !device && (
                <p className="account-note">演示模式只展示设备码界面，真实登录需在桌面客户端中完成。</p>
              )}
            </div>
          )}

          {tab === 'authlib' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon authlib"><Globe2 size={22} /></div>
                <div>
                  <h3>外置登录</h3>
                  <p>支持 LittleSkin 与 YggdrasilSkinServer，也兼容其他 Authlib-Injector 服务器。</p>
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
              <label className="field">
                <span className="field-label"><Server size={14} /> 认证服务器地址</span>
                <input
                  className="text-input mono"
                  value={authServer}
                  onChange={(event) => setAuthServer(event.target.value)}
                  placeholder="https://littleskin.cn/api/yggdrasil"
                />
              </label>
              <div className="settings-grid-2">
                <label className="field">
                  <span className="field-label"><UserRound size={14} /> 用户名 / 邮箱</span>
                  <input className="text-input" value={authUser} onChange={(event) => setAuthUser(event.target.value)} autoComplete="username" />
                </label>
                <label className="field">
                  <span className="field-label"><KeyRound size={14} /> 密码</span>
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
                  <div><strong>外置登录失败</strong><span>{authError}</span></div>
                </div>
              )}
              <button type="button" className="btn btn-primary" onClick={() => void loginAuthlib()} disabled={authBusy || !authUser || !authPassword}>
                {authBusy ? <LoaderCircle size={16} className="spin" /> : <LogIn size={16} />}
                {authBusy ? '正在验证…' : '登录外置账户'}
              </button>
              <p className="account-note">密码只用于本次认证，不会保存。访问令牌保存在本机配置中。</p>
            </div>
          )}

          {tab === 'offline' && (
            <div className="account-form">
              <div className="account-brand-head">
                <div className="account-brand-icon offline"><UserRound size={22} /></div>
                <div>
                  <h3>{editingOfflineId ? '重命名离线账户' : '添加离线账户'}</h3>
                  <p>离线账户只能用于本地游戏和未开启正版验证的服务器。</p>
                </div>
              </div>
              <label className="field">
                <span className="field-label">离线玩家名 <small>最多 16 个字符</small></span>
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
                  {editingOfflineId ? '保存名称' : '添加离线账户'}
                </button>
                {editingOfflineId && (
                  <button type="button" className="btn btn-ghost" onClick={() => { setEditingOfflineId(null); setOfflineName(''); }}>
                    新建离线账户
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
