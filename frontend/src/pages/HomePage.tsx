import { useCallback, useEffect, useRef, useState } from 'react';
import {
  Boxes,
  Coffee,
  Cpu,
  Layers3,
  LoaderCircle,
  Play,
  Plus,
  Quote,
  RefreshCw,
  Settings2,
  Square,
  TerminalSquare,
  Wrench,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { InstanceCard } from '../components/InstanceCard';
import { EmptyState } from '../components/EmptyState';
import { greetingKey, loaderLabel, loaderTone, relativeTime } from '../lib/format';
import type { Hitokoto } from '../lib/types';

const HITOKOTO_ROTATE_MS = 5000;

export function HomePage() {
  const {
    instances, selectedInstance, selectedId, selectInstance, settings, selectedAccount,
    navigate, openCreate, openEdit, askDelete, launch, stop, openConsole, openInstanceFolder,
    t, locale, launchingId, hasFeature, openLicense, pushToast, refreshInstances,
  } = useApp();
  const [repairingId, setRepairingId] = useState<string | null>(null);
  const [hitokoto, setHitokoto] = useState<Hitokoto | null>(null);
  const [hitokotoLoading, setHitokotoLoading] = useState(true);
  const [hitokotoError, setHitokotoError] = useState('');
  const rotating = useRef(false);

  const loadHitokoto = useCallback(async () => {
    setHitokotoLoading(true);
    setHitokotoError('');
    try {
      setHitokoto(await api.getHitokoto());
    } catch (error) {
      setHitokotoError((error as Error).message);
    } finally {
      setHitokotoLoading(false);
    }
  }, []);

  // Background refresh for the five-second rotation. It deliberately does not
  // toggle the loading state, so the quote area never flickers.
  const rotateHitokoto = useCallback(async () => {
    if (rotating.current) return;
    rotating.current = true;
    try {
      const next = await api.getHitokoto();
      setHitokoto((current) => (next.text && next.text !== current?.text ? next : current ?? next));
      setHitokotoError('');
    } catch {
      // Keep the current quote when a background refresh fails.
    } finally {
      rotating.current = false;
    }
  }, []);

  useEffect(() => {
    void loadHitokoto();
  }, [loadHitokoto]);

  const repairVersion = async (instanceId: string) => {
    if (!hasFeature('versions')) {
      openLicense();
      return;
    }
    setRepairingId(instanceId);
    try {
      await api.repairVersion(instanceId);
      pushToast('success', t('instanceSettings.repairDone'));
      await refreshInstances();
    } catch (reason) {
      pushToast('error', t('instanceSettings.repairFailed'), (reason as Error).message);
    } finally {
      setRepairingId(null);
    }
  };

  useEffect(() => {
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') void rotateHitokoto();
    }, HITOKOTO_ROTATE_MS);
    return () => window.clearInterval(timer);
  }, [rotateHitokoto]);

  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';
  const running = instances.filter((instance) => instance.running);
  const hitokotoSource = hitokoto
    ? [hitokoto.from, hitokoto.fromWho].filter(Boolean).join(' · ') || t('home.hitokotoTag')
    : '';

  return (
    <div className="page home-page">
      <section className="home-head">
        <div>
          <div className="eyebrow">
            <span className="eyebrow-dash" />
            {t('home.eyebrow')} · {t('home.greeting', { greeting: t(greetingKey()), player })}
          </div>
          <h1 className="home-title">{t('home.title')}</h1>
          <p>{t('home.subtitle')}</p>
        </div>
        <button type="button" className="btn btn-primary" onClick={openCreate}>
          <Plus size={17} /> {t('home.newVersion')}
        </button>
      </section>

      {selectedInstance ? (
        <section className={`play-panel tone-${loaderTone(selectedInstance.loader)}`}>
          <div className="play-copy">
            <div className="play-title">
              <span className="eyebrow">{t('home.currentVersion')}</span>
              {selectedInstance.running && (
                <span className="running-pill"><span className="live-pulse" />{t('home.running')}</span>
              )}
            </div>
            <h2>{selectedInstance.name}</h2>
            <div className="meta-row">
              <span className="meta-chip"><Boxes size={13} /> {selectedInstance.mcVersion}</span>
              <span className="meta-chip"><Layers3 size={13} /> {loaderLabel(selectedInstance.loader, t('loader.vanilla'))}</span>
              <span className="meta-chip"><Coffee size={13} /> Java {selectedInstance.requiredJava}</span>
              <span className="meta-chip">
                <Cpu size={13} />
                {((selectedInstance.effectiveMemoryMB || 0) / 1024).toFixed(0)} GB
              </span>
            </div>
            <p className="play-hint">
              {selectedInstance.versionInstalled
                ? t('home.lastLaunch', { time: relativeTime(selectedInstance.lastPlayed, locale, t('common.unknown')) })
                : t('home.filesIncomplete')}
            </p>
          </div>
          <div className="play-actions">
            {selectedInstance.running ? (
              <>
                <button type="button" className="btn btn-primary btn-lg" onClick={() => openConsole(selectedInstance.id)}>
                  <TerminalSquare size={18} /> {t('common.openConsole')}
                </button>
                <button type="button" className="btn btn-danger-ghost" onClick={() => void stop(selectedInstance.id)}>
                  <Square size={14} fill="currentColor" /> {t('common.stopGame')}
                </button>
              </>
            ) : (
              <>
                <button
                  type="button"
                  className="btn btn-primary btn-lg"
                  onClick={() => void launch(selectedInstance.id)}
                  disabled={launchingId === selectedInstance.id}
                >
                  {launchingId === selectedInstance.id
                    ? <LoaderCircle size={18} className="spin" />
                    : <Play size={18} fill="currentColor" />}
                  {launchingId === selectedInstance.id ? t('launch.button') : t('home.launchGame')}
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => openEdit(selectedInstance.id)}>
                  <Settings2 size={16} /> {t('common.versionSettings')}
                </button>
                <button
                  type="button"
                  className="btn btn-ghost"
                  disabled={repairingId === selectedInstance.id}
                  title={t('instanceSettings.repairHint')}
                  onClick={() => void repairVersion(selectedInstance.id)}
                >
                  {repairingId === selectedInstance.id
                    ? <LoaderCircle size={16} className="spin" />
                    : <Wrench size={16} />}
                  {repairingId === selectedInstance.id
                    ? t('instanceSettings.repairing')
                    : t('instanceSettings.repair')}
                </button>
              </>
            )}
          </div>
        </section>
      ) : (
        <section className="play-panel play-panel-empty">
          <div>
            <span className="eyebrow">{t('home.currentVersion')}</span>
            <h2>{t('home.noVersionsTitle')}</h2>
            <p>{t('home.noVersionsBody')}</p>
          </div>
          <div className="play-actions">
            <button type="button" className="btn btn-primary" onClick={openCreate}>
              <Plus size={16} /> {t('home.newVersion')}
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => navigate('versions')}>
              {t('home.addLocalVersion')}
            </button>
          </div>
        </section>
      )}

      <section className="hitokoto-card">
        <div className="hitokoto-head">
          <span className="eyebrow"><span className="eyebrow-dash" />{t('home.didYouKnow')}</span>
          <button
            type="button"
            className="icon-button icon-button-sm"
            onClick={() => void loadHitokoto()}
            disabled={hitokotoLoading}
            title={t('home.refreshQuote')}
            aria-label={t('home.refreshQuote')}
          >
            <RefreshCw size={14} className={hitokotoLoading ? 'spin' : ''} />
          </button>
        </div>
        {hitokoto ? (
          <div className="hitokoto-rotator" key={hitokoto.text}>
            <p className="hitokoto-text">“{hitokoto.text}”</p>
            <div className="hitokoto-meta">
              <span>—— {hitokotoSource}</span>
              <span className="hitokoto-tag"><Quote size={11} /> {t('home.hitokotoTag')}</span>
            </div>
          </div>
        ) : hitokotoLoading ? (
          <div className="hitokoto-skeleton">
            <span className="skeleton skeleton-line wide" />
            <span className="skeleton skeleton-line short" />
          </div>
        ) : (
          <p className="hitokoto-text is-error" title={hitokotoError}>
            {t('home.hitokotoError')}
          </p>
        )}
      </section>

      <section>
        <div className="section-head">
          <div>
            <h2>{t('home.myVersions')} <span className="section-count">{instances.length}</span></h2>
            {running.length > 0 && <span className="section-subtitle">{t('home.runningCount', { count: running.length })}</span>}
          </div>
          <div className="section-actions">
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => navigate('versions')}>
              <Boxes size={15} /> {t('home.addVersion')}
            </button>
            <button type="button" className="btn btn-primary btn-sm" onClick={openCreate}>
              <Plus size={15} /> {t('home.newShort')}
            </button>
          </div>
        </div>

        {instances.length === 0 ? (
          <EmptyState
            icon={<Boxes size={26} />}
            title={t('home.noneTitle')}
            body={t('home.noneBody')}
            action={<button type="button" className="btn btn-primary" onClick={() => navigate('versions')}><Plus size={16} /> {t('home.addVersion')}</button>}
          />
        ) : (
          <div className="instance-list">
            {instances.map((instance) => (
              <InstanceCard
                key={instance.id}
                instance={instance}
                active={instance.id === selectedId}
                onSelect={() => selectInstance(instance.id)}
                onLaunch={() => void launch(instance.id)}
                onStop={() => void stop(instance.id)}
                onConsole={() => openConsole(instance.id)}
                onFolder={() => void openInstanceFolder(instance.id)}
                onEdit={() => openEdit(instance.id)}
                onDelete={() => askDelete(instance.id)}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
