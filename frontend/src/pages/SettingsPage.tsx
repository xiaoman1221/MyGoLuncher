import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import {
  BadgeCheck,
  Coffee,
  Cpu,
  FolderOpen,
  Globe2,
  Info,
  Moon,
  Quote,
  Save,
  Sun,
  UserRound,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import type { Settings } from '../lib/types';
import { cn } from '../lib/format';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384, 32768];

export function SettingsPage() {
  const {
    settings, bootstrap, accounts, selectedAccount, saveSettings, pushToast,
    openDataFolder, openAccounts, isDemo, theme, setTheme,
  } = useApp();
  const [form, setForm] = useState<Settings | null>(settings ? { ...settings } : null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (settings) setForm({ ...settings });
  }, [settings]);

  const dirty = useMemo(() => {
    if (!form || !settings) return false;
    return JSON.stringify(form) !== JSON.stringify(settings);
  }, [form, settings]);

  if (!form) {
    return <div className="page"><div className="skeleton skeleton-line wide" /></div>;
  }

  const patch = <K extends keyof Settings>(key: K, value: Settings[K]) => {
    setForm((current) => current ? { ...current, [key]: value } : current);
  };

  const save = async () => {
    if (form.defaultMemoryMB !== 0 && form.defaultMemoryMB < 512) {
      pushToast('error', '默认内存必须为自动或至少 512 MB');
      return;
    }
    setSaving(true);
    try {
      await saveSettings(form);
      pushToast('success', '设置已保存');
    } catch (error) {
      pushToast('error', '设置保存失败', (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="page settings-page">
      <PageHeader
        kicker="SETTINGS"
        title={<>设置 <span className="title-accent">/</span> MyGoLuncher</>}
        description="主题、账户、内存、Java 与首页一言。其余选项保持默认即可。"
        actions={
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving || !dirty}>
            <Save size={15} />
            {saving ? '正在保存…' : dirty ? '保存设置' : '已保存'}
          </button>
        }
      />

      {isDemo && (
        <div className="inline-banner banner-warning">
          <Info size={17} />
          <div>
            <strong>浏览器演示模式</strong>
            <span>文件选择、下载与启动游戏仅在桌面客户端中可用。</span>
          </div>
        </div>
      )}

      <section className="panel">
        <div className="panel-head">
          <h2>界面主题</h2>
          <p>黑夜模式使用荧光绿，白天模式使用粉色作为主色。</p>
        </div>
        <div className="theme-options">
          <button
            type="button"
            className={cn('theme-option', theme === 'dark' && 'is-active')}
            onClick={() => setTheme('dark')}
          >
            <div className="theme-swatch theme-swatch-dark">
              <span className="swatch-bar" />
              <span className="swatch-card" />
              <span className="swatch-dot" />
            </div>
            <span className="theme-option-copy">
              <Moon size={15} />
              <strong>黑夜模式</strong>
              <small>深色界面 · 荧光绿</small>
            </span>
          </button>
          <button
            type="button"
            className={cn('theme-option', theme === 'light' && 'is-active')}
            onClick={() => setTheme('light')}
          >
            <div className="theme-swatch theme-swatch-light">
              <span className="swatch-bar" />
              <span className="swatch-card" />
              <span className="swatch-dot" />
            </div>
            <span className="theme-option-copy">
              <Sun size={15} />
              <strong>白天模式</strong>
              <small>浅色界面 · 粉色主色</small>
            </span>
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2>账户</h2>
            <p>管理 Microsoft 正版、外置登录与离线身份。</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={openAccounts}>管理账户</button>
        </div>
        <div className="account-summary">
          <div className={`account-summary-mark kind-${selectedAccount?.kind ?? 'offline'}`}>
            {selectedAccount?.kind === 'microsoft'
              ? <BadgeCheck size={20} />
              : selectedAccount?.kind === 'authlib'
                ? <Globe2 size={20} />
                : <UserRound size={20} />}
          </div>
          <div className="account-summary-copy">
            <span>当前身份</span>
            <strong>{selectedAccount?.name ?? form.playerName}</strong>
            <small>{selectedAccount?.label ?? '离线'}{selectedAccount?.serverUrl ? ` · ${selectedAccount.serverUrl}` : ''}</small>
          </div>
          <span className="account-total">共 {accounts.length} 个身份</span>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>游戏设置</h2>
          <p>默认内存与 Java 会应用到新建版本，已有版本可在版本设置中单独调整。</p>
        </div>
        <div className="settings-fields">
          <div className="field">
            <div className="field-label-row">
              <span className="field-label"><Cpu size={14} /> 默认内存</span>
              <span className="memory-value">
                {form.defaultMemoryMB === 0
                  ? `自动 · 约 ${((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1)} GB`
                  : `${(form.defaultMemoryMB / 1024).toFixed(1)} GB`}
              </span>
            </div>
            <label className="toggle-row">
              <span>
                <strong>自动分配内存</strong>
                <small>根据物理内存计算推荐值，并为系统保留余量</small>
              </span>
              <input
                type="checkbox"
                className="switch"
                checked={form.defaultMemoryMB === 0}
                onChange={(event) => patch('defaultMemoryMB', event.target.checked ? 0 : (bootstrap?.autoMemoryMB ?? 4096))}
              />
            </label>
            {form.defaultMemoryMB !== 0 && (
              <>
                <input
                  className="range"
                  type="range"
                  min={1024}
                  max={32768}
                  step={512}
                  value={form.defaultMemoryMB}
                  onChange={(event) => patch('defaultMemoryMB', Number(event.target.value))}
                  style={{ '--range-progress': `${((form.defaultMemoryMB - 1024) / (32768 - 1024)) * 100}%` } as CSSProperties}
                />
                <div className="memory-presets">
                  {memorySteps.map((value) => (
                    <button
                      key={value}
                      type="button"
                      className={cn('chip', 'chip-button', form.defaultMemoryMB === value && 'is-active')}
                      onClick={() => patch('defaultMemoryMB', value)}
                    >
                      {value / 1024} GB
                    </button>
                  ))}
                </div>
              </>
            )}
          </div>

          <label className="field">
            <span className="field-label"><Coffee size={14} /> 默认 Java 路径</span>
            <input
              className="text-input mono"
              value={form.defaultJavaPath}
              onChange={(event) => patch('defaultJavaPath', event.target.value)}
              placeholder="留空自动匹配，或填写 java.exe / JAVA_HOME"
            />
            <span className="field-hint">版本级 Java 设置优先于这里。留空时会在启动前自动寻找或下载匹配的 Java。</span>
          </label>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>首页一言</h2>
          <p>首页“你知道吗”的内容来源，默认使用 HitokotoGo。</p>
        </div>
        <label className="field">
          <span className="field-label"><Quote size={14} /> 一言 API 地址</span>
          <input
            className="text-input mono"
            value={form.hitokotoApiUrl ?? ''}
            onChange={(event) => patch('hitokotoApiUrl', event.target.value)}
            placeholder="https://hitokotogo.yhdzz.cn/v2"
          />
          <span className="field-hint">
            接口需返回包含 <code>hitokoto</code> 字段的 JSON。
            <button type="button" className="text-button" onClick={() => void api.openURL('https://hitokotogo.yhdzz.cn/docs')}>
              查看文档
            </button>
            <button
              type="button"
              className="text-button"
              onClick={() => patch('hitokotoApiUrl', 'https://hitokotogo.yhdzz.cn/v2')}
            >
              恢复默认
            </button>
          </span>
        </label>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>目录</h2>
          <p>游戏数据与启动器配置都保存在程序目录内。</p>
        </div>
        <div className="path-list">
          <div className="path-row">
            <span>游戏目录</span>
            <code title={bootstrap?.paths.data}>{bootstrap?.paths.data ?? '—'}</code>
          </div>
          <div className="path-row">
            <span>配置目录</span>
            <code title={bootstrap?.paths.root}>{bootstrap?.paths.root ?? '—'}</code>
          </div>
        </div>
        <div className="panel-actions">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void openDataFolder()}>
            <FolderOpen size={15} /> 打开游戏目录
          </button>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void api.openConfigFolder()}>
            <FolderOpen size={15} /> 打开配置目录
          </button>
        </div>
      </section>
    </div>
  );
}
