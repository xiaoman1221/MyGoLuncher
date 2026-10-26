import { useEffect, useMemo, useRef, useState, type CSSProperties } from 'react';
import {
  BadgeCheck,
  Coffee,
  Cpu,
  FolderOpen,
  Globe2,
  Info,
  Link2,
  LoaderCircle,
  Minimize2,
  Moon,
  Power,
  Quote,
  Save,
  Sun,
  UserRound,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { DownloadMirrorSettings, ResourceMirrorSettings } from '../components/MirrorSettings';
import { UpdatePanel } from '../components/UpdatePanel';
import { LicensePanel } from '../components/LicensePanel';
import type { Settings } from '../lib/types';
import {
  accountKindKey,
  languageNames,
  normalizeLanguage,
  resolveStartupLanguage,
  type Language,
} from '../lib/i18n';
import { cn } from '../lib/format';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384, 32768];

export function SettingsPage() {
  const {
    settings, bootstrap, accounts, selectedAccount, saveSettings, pushToast,
    openDataFolder, openAccounts, isDemo, theme, setTheme, t, language, setLanguage,
    reloadBootstrap,
  } = useApp();
  const [form, setForm] = useState<Settings | null>(settings ? { ...settings } : null);
  const [saving, setSaving] = useState(false);
  const [orcaBusy, setOrcaBusy] = useState(false);
  const orcaCancelled = useRef(false);

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
      pushToast('error', t('settings.memoryInvalid'));
      return;
    }
    setSaving(true);
    try {
      await saveSettings(form);
      setLanguage(resolveStartupLanguage(form.languageAuto, form.language));
      pushToast('success', t('settings.savedToast'));
    } catch (error) {
      pushToast('error', t('settings.saveFailed'), (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const changeLanguage = (value: string) => {
    if (value === 'auto') {
      patch('languageAuto', true);
      return;
    }
    patch('languageAuto', false);
    patch('language', normalizeLanguage(value));
  };

  const aiProvider = form.aiSearchProvider === 'anthropic'
    ? 'anthropic'
    : form.aiSearchProvider === 'orcarouter' ? 'orcarouter' : 'deepseek';
  const isAnthropic = aiProvider === 'anthropic';
  const isOrcaRouter = aiProvider === 'orcarouter';
  const aiModelPlaceholder = isAnthropic
    ? 'claude-opus-5-5'
    : isOrcaRouter ? 'orcarouter/auto' : 'deepseek-chat';
  const aiBasePlaceholder = isAnthropic
    ? 'https://api.anthropic.com'
    : isOrcaRouter ? 'https://api.orcarouter.ai/v1' : 'https://api.deepseek.com';
  const aiKeyPlaceholder = isAnthropic ? 'sk-ant-...' : isOrcaRouter ? 'OrcaRouter API Key' : 'sk-...';
  const aiKeyURL = isAnthropic
    ? 'https://console.anthropic.com/settings/keys'
    : isOrcaRouter ? 'https://www.orcarouter.ai' : 'https://platform.deepseek.com/api_keys';

  const changeAIProvider = (provider: string) => {
    const next = provider === 'anthropic'
      ? 'anthropic'
      : provider === 'orcarouter' ? 'orcarouter' : 'deepseek';
    const currentModel = (form.aiSearchModel ?? '').trim();
    const knownModels = ['deepseek-chat', 'deepseek-reasoner', 'claude-opus-5-5', 'orcarouter/auto'];
    patch('aiSearchProvider', next);
    if (next !== aiProvider) {
      patch('aiSearchApiKey', '');
    }
    if (!currentModel || knownModels.includes(currentModel)) {
      patch('aiSearchModel', next === 'anthropic'
        ? 'claude-opus-5-5'
        : next === 'orcarouter' ? 'orcarouter/auto' : 'deepseek-chat');
    }
    const currentBase = (form.aiSearchBaseUrl ?? '').trim().replace(/\/+$/, '');
    if (!currentBase || [
      'https://api.anthropic.com',
      'https://api.deepseek.com',
      'https://api.deepseek.com/v1',
      'https://api.orcarouter.ai/v1',
    ].includes(currentBase)) {
      patch('aiSearchBaseUrl', '');
    }
  };

  const authorizeOrcaRouter = async () => {
    orcaCancelled.current = false;
    setOrcaBusy(true);
    try {
      await api.beginOrcaRouterLogin();
      await reloadBootstrap();
      pushToast('success', t('settings.orcaSuccess'));
    } catch (error) {
      if (!orcaCancelled.current) {
        pushToast('error', t('settings.orcaFailed'), (error as Error).message);
      }
    } finally {
      setOrcaBusy(false);
      orcaCancelled.current = false;
    }
  };

  const cancelOrcaRouter = async () => {
    orcaCancelled.current = true;
    try {
      await api.cancelOrcaRouterLogin();
    } catch {
      // The pending request reports the cancellation.
    }
  };

  return (
    <div className="page settings-page">
      <PageHeader
        kicker="SETTINGS"
        title={<>{t('settings.title')} <span className="title-accent">/</span> MyGoLuncher</>}
        description={t('settings.description')}
        actions={
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving || orcaBusy || !dirty}>
            <Save size={15} />
            {saving ? t('common.saving') : dirty ? t('common.saveSettings') : t('common.saved')}
          </button>
        }
      />

      {isDemo && (
        <div className="inline-banner banner-warning">
          <Info size={17} />
          <div>
            <strong>{t('settings.demoTitle')}</strong>
            <span>{t('settings.demoBody')}</span>
          </div>
        </div>
      )}

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.themeTitle')}</h2>
          <p>{t('settings.themeBody')}</p>
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
              <strong>{t('settings.darkTitle')}</strong>
              <small>{t('settings.darkDesc')}</small>
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
              <strong>{t('settings.lightTitle')}</strong>
              <small>{t('settings.lightDesc')}</small>
            </span>
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.languageTitle')}</h2>
          <p>{t('settings.languageBody')}</p>
        </div>
        <label className="field">
          <span className="field-label">{t('settings.languageStartup')}</span>
          <select
            className="text-input"
            value={form.languageAuto ? 'auto' : form.language}
            onChange={(event) => changeLanguage(event.target.value)}
          >
            <option value="auto">{t('settings.languageAuto')}</option>
            {(Object.keys(languageNames) as Language[]).map((code) => (
              <option key={code} value={code}>{languageNames[code]}</option>
            ))}
          </select>
          <span className="field-hint">{t('settings.languageHint')}</span>
          <span className="field-hint">{t('settings.currentLanguage', { language: languageNames[language] })}</span>
        </label>
      </section>

      <section className="panel">
        <div className="panel-head panel-head-row">
          <div>
            <h2>{t('settings.accountTitle')}</h2>
            <p>{t('settings.accountBody')}</p>
          </div>
          <button type="button" className="btn btn-ghost btn-sm" onClick={openAccounts}>{t('settings.manageAccounts')}</button>
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
            <span>{t('settings.currentIdentity')}</span>
            <strong>{selectedAccount?.name ?? form.playerName}</strong>
            <small>{t(accountKindKey(selectedAccount?.kind ?? 'offline'))}{selectedAccount?.serverUrl ? ` · ${selectedAccount.serverUrl}` : ''}</small>
          </div>
          <span className="account-total">{t('settings.identityCount', { count: accounts.length })}</span>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.gameTitle')}</h2>
          <p>{t('settings.gameBody')}</p>
        </div>
        <div className="settings-fields">
          <div className="field">
            <div className="field-label-row">
              <span className="field-label"><Cpu size={14} /> {t('settings.defaultMemory')}</span>
              <span className="memory-value">
                {form.defaultMemoryMB === 0
                  ? t('settings.autoMemoryValue', { gb: ((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1) })
                  : `${(form.defaultMemoryMB / 1024).toFixed(1)} GB`}
              </span>
            </div>
            <label className="toggle-row">
              <span>
                <strong>{t('settings.autoMemory')}</strong>
                <small>{t('settings.autoMemoryHint')}</small>
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
            <span className="field-label"><Coffee size={14} /> {t('settings.defaultJava')}</span>
            <input
              className="text-input mono"
              value={form.defaultJavaPath}
              onChange={(event) => patch('defaultJavaPath', event.target.value)}
              placeholder={t('settings.javaPlaceholder')}
            />
            <span className="field-hint">{t('settings.javaHint')}</span>
          </label>

          <div className="field">
            <label className="toggle-row">
              <span>
                <strong>{t('settings.versionIsolation')}</strong>
                <small>{t('settings.versionIsolationHint')}</small>
              </span>
              <input
                type="checkbox"
                className="switch"
                checked={form.versionIsolation !== false}
                onChange={(event) => patch('versionIsolation', event.target.checked)}
              />
            </label>
            <span className="field-hint">
              {form.versionIsolation !== false ? t('settings.versionIsolationOn') : t('settings.versionIsolationOff')}
            </span>
          </div>

          <div className="field">
            <label className="toggle-row">
              <span>
                <strong>{t('settings.serverBackup')}</strong>
                <small>{t('settings.serverBackupHint')}</small>
              </span>
              <input
                type="checkbox"
                className="switch"
                checked={form.serverBackupDisabled !== true}
                onChange={(event) => patch('serverBackupDisabled', !event.target.checked)}
              />
            </label>
            <span className="field-hint">{t('settings.serverBackupNote')}</span>
          </div>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.windowTitle')}</h2>
          <p>{t('settings.windowBody')}</p>
        </div>
        <div className="window-choice-grid" role="radiogroup" aria-label={t('settings.closeBehavior')}>
          <button
            type="button"
            role="radio"
            aria-checked={(form.closeBehavior ?? 'ask') === 'ask'}
            className={cn('window-choice', (form.closeBehavior ?? 'ask') === 'ask' && 'is-active')}
            onClick={() => patch('closeBehavior', 'ask')}
          >
            <Info size={18} />
            <span>
              <strong>{t('settings.closeAsk')}</strong>
              <small>{t('settings.closeAskHint')}</small>
            </span>
          </button>
          <button
            type="button"
            role="radio"
            aria-checked={form.closeBehavior === 'minimize'}
            className={cn('window-choice', form.closeBehavior === 'minimize' && 'is-active')}
            onClick={() => patch('closeBehavior', 'minimize')}
          >
            <Minimize2 size={18} />
            <span>
              <strong>{t('settings.closeMinimize')}</strong>
              <small>{t('settings.closeMinimizeHint')}</small>
            </span>
          </button>
          <button
            type="button"
            role="radio"
            aria-checked={form.closeBehavior === 'exit'}
            className={cn('window-choice', form.closeBehavior === 'exit' && 'is-active')}
            onClick={() => patch('closeBehavior', 'exit')}
          >
            <Power size={18} />
            <span>
              <strong>{t('settings.closeExit')}</strong>
              <small>{t('settings.closeExitHint')}</small>
            </span>
          </button>
        </div>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.gameMirrorTitle')}</h2>
          <p>{t('settings.gameMirrorBody')}</p>
        </div>
        <DownloadMirrorSettings form={form} patch={patch} t={t} />
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.resourceMirrorTitle')}</h2>
          <p>{t('settings.resourceMirrorBody')}</p>
        </div>
        <ResourceMirrorSettings form={form} patch={patch} t={t} />
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.curseForgeTitle')}</h2>
          <p>{t('settings.curseForgeBody')}</p>
        </div>
        <label className="field">
          <span className="field-label">{t('settings.curseForgeKey')}</span>
          <input
            className="text-input mono"
            type="password"
            value={form.curseForgeAPIKey ?? ''}
            onChange={(event) => patch('curseForgeAPIKey', event.target.value)}
            placeholder="$2a$10$..."
            autoComplete="off"
          />
          <span className="field-hint">{t('settings.curseForgeHint')}</span>
        </label>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.aiTitle')}</h2>
          <p>{t('settings.aiBody')}</p>
        </div>
        <div className="settings-fields">
          <label className="toggle-row">
            <span>
              <strong>{t('settings.aiEnabled')}</strong>
              <small>{t('settings.aiEnabledHint')}</small>
            </span>
            <input
              type="checkbox"
              className="switch"
              checked={form.aiSearchEnabled === true}
              onChange={(event) => patch('aiSearchEnabled', event.target.checked)}
            />
          </label>
          <label className="field">
            <span className="field-label">{t('settings.aiProvider')}</span>
            <select
              className="text-input"
              value={aiProvider}
              onChange={(event) => changeAIProvider(event.target.value)}
            >
              <option value="deepseek">{t('settings.aiProviderDeepSeek')}</option>
              <option value="anthropic">{t('settings.aiProviderAnthropic')}</option>
              <option value="orcarouter">{t('settings.aiProviderOrcaRouter')}</option>
            </select>
            <span className="field-hint">{t('settings.aiProviderHint')}</span>
          </label>
          <label className="field">
            <span className="field-label">{t('settings.aiModel')}</span>
            <input
              className="text-input mono"
              value={form.aiSearchModel ?? ''}
              onChange={(event) => patch('aiSearchModel', event.target.value)}
              placeholder={aiModelPlaceholder}
            />
          </label>
          <label className="field">
            <span className="field-label">{t('settings.aiAPIKey')}</span>
            <input
              className="text-input mono"
              type="password"
              value={form.aiSearchApiKey ?? ''}
              onChange={(event) => patch('aiSearchApiKey', event.target.value)}
              placeholder={aiKeyPlaceholder}
              autoComplete="off"
            />
          </label>
          {isOrcaRouter && (
            <div className="field orca-auth-field">
              <div className="orca-auth-row">
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  onClick={() => void authorizeOrcaRouter()}
                  disabled={orcaBusy}
                >
                  {orcaBusy ? <LoaderCircle size={14} className="spin" /> : <Link2 size={14} />}
                  {orcaBusy ? t('settings.orcaWaiting') : t('settings.orcaAuthorize')}
                </button>
                {orcaBusy && (
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void cancelOrcaRouter()}>
                    {t('settings.orcaCancel')}
                  </button>
                )}
              </div>
              <span className="field-hint">{t('settings.orcaHint')}</span>
              <span className="field-hint">{t('settings.orcaConfigHint')}</span>
            </div>
          )}
          <label className="field">
            <span className="field-label">{t('settings.aiBaseURL')}</span>
            <input
              className="text-input mono"
              value={form.aiSearchBaseUrl ?? ''}
              onChange={(event) => patch('aiSearchBaseUrl', event.target.value)}
              placeholder={aiBasePlaceholder}
            />
            <span className="field-hint">{t('settings.aiBaseURLHint')}</span>
          </label>
          <div className="panel-actions">
            <button type="button" className="text-button" onClick={() => void api.openURL(aiKeyURL)}>
              {t('settings.aiConsole')}
            </button>
          </div>
        </div>
      </section>

      <LicensePanel />

      <UpdatePanel />

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.hitokotoTitle')}</h2>
          <p>{t('settings.hitokotoBody')}</p>
        </div>
        <label className="field">
          <span className="field-label"><Quote size={14} /> {t('settings.hitokotoAPI')}</span>
          <input
            className="text-input mono"
            value={form.hitokotoApiUrl ?? ''}
            onChange={(event) => patch('hitokotoApiUrl', event.target.value)}
            placeholder="https://hitokotogo.yhdzz.cn/v2"
          />
          <span className="field-hint">
            {t('settings.hitokotoHint')}
            <button type="button" className="text-button" onClick={() => void api.openURL('https://hitokotogo.yhdzz.cn/docs')}>
              {t('settings.viewDocs')}
            </button>
            <button
              type="button"
              className="text-button"
              onClick={() => patch('hitokotoApiUrl', 'https://hitokotogo.yhdzz.cn/v2')}
            >
              {t('settings.restoreDefault')}
            </button>
          </span>
        </label>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('settings.dirsTitle')}</h2>
          <p>{t('settings.dirsBody')}</p>
        </div>
        <div className="path-list">
          <div className="path-row">
            <span>{t('settings.gameDir')}</span>
            <code title={bootstrap?.paths.data}>{bootstrap?.paths.data ?? '—'}</code>
          </div>
          <div className="path-row">
            <span>{t('settings.configDir')}</span>
            <code title={bootstrap?.paths.root}>{bootstrap?.paths.root ?? '—'}</code>
          </div>
        </div>
        <div className="panel-actions">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void openDataFolder()}>
            <FolderOpen size={15} /> {t('common.openGameFolder')}
          </button>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void api.openConfigFolder()}>
            <FolderOpen size={15} /> {t('settings.openConfigDir')}
          </button>
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={() => {
              void api.syncLauncherProfiles()
                .then(() => pushToast('success', t('settings.portableDone')))
                .catch((error: Error) => pushToast('error', t('settings.portableFailed'), error.message));
            }}
          >
            <Globe2 size={15} /> {t('settings.portableSync')}
          </button>
        </div>
      </section>
    </div>
  );
}
