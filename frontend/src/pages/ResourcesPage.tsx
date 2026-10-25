import { useEffect, useMemo, useState } from 'react';
import {
  Boxes,
  Database,
  Image,
  LoaderCircle,
  LockKeyhole,
  PackageOpen,
  RefreshCw,
  Search,
  Sparkles,
  TriangleAlert,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { ModUpdatesModal } from '../components/ModUpdatesModal';
import type {
  InstanceView,
  ResourceMirror,
  ResourceProject,
  ResourceSearchResult,
  ResourceType,
} from '../lib/types';
import { cn, formatCompactNumber, initials } from '../lib/format';

const tabs: Array<{ key: ResourceType; labelKey: 'resources.type.mod' | 'resources.type.shader' | 'resources.type.resourcepack' | 'resources.type.datapack' | 'resources.type.modpack'; icon: typeof PackageOpen }> = [
  { key: 'mod', labelKey: 'resources.type.mod', icon: PackageOpen },
  { key: 'shader', labelKey: 'resources.type.shader', icon: Sparkles },
  { key: 'resourcepack', labelKey: 'resources.type.resourcepack', icon: Image },
  { key: 'datapack', labelKey: 'resources.type.datapack', icon: Database },
  { key: 'modpack', labelKey: 'resources.type.modpack', icon: Boxes },
];

export function ResourcesPage() {
  const {
    instances, settings, selectedId, refreshInstances, selectInstance, navigate,
    pushToast, t, locale, hasFeature, openLicense,
  } = useApp();
  const [type, setType] = useState<ResourceType>('mod');
  const [targetId, setTargetId] = useState(selectedId ?? instances[0]?.id ?? '');
  const [query, setQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [result, setResult] = useState<ResourceSearchResult | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [installing, setInstalling] = useState<string | null>(null);
  const aiEnabled = settings?.aiSearchEnabled === true;
  const aiEntitled = hasFeature('ai');
  const [useAI, setUseAI] = useState(aiEnabled);
  const [mirrors, setMirrors] = useState<ResourceMirror[]>([]);
  const [mirrorId, setMirrorId] = useState(settings?.resourceMirror || 'official');
  const [updatesTarget, setUpdatesTarget] = useState<InstanceView | null>(null);

  useEffect(() => {
    let alive = true;
    api.listResourceMirrors()
      .then((rows) => { if (alive) setMirrors(rows); })
      .catch(() => { if (alive) setMirrors([]); });
    return () => { alive = false; };
  }, []);

  useEffect(() => {
    if (settings?.resourceMirror) setMirrorId(settings.resourceMirror);
  }, [settings?.resourceMirror]);

  const needsInstance = type !== 'modpack';
  const hasInstance = instances.some((instance) => instance.id === targetId);

  useEffect(() => {
    if (hasInstance) return;
    setTargetId(instances[0]?.id ?? '');
  }, [hasInstance, instances]);

  useEffect(() => {
    setUseAI(aiEnabled);
  }, [aiEnabled]);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(query.trim()), 350);
    return () => window.clearTimeout(timer);
  }, [query]);

  useEffect(() => {
    if (needsInstance && !targetId) {
      setResult(null);
      setLoading(false);
      return;
    }
    let alive = true;
    setLoading(true);
    setError('');
    api.searchResources({
      instanceId: targetId,
      query: debouncedQuery,
      type,
      source: 'all',
      sort: debouncedQuery ? 'relevance' : 'downloads',
      offset: 0,
      limit: 18,
      useAI: aiEnabled && aiEntitled && useAI,
    })
      .then((found) => {
        if (alive) setResult(found);
      })
      .catch((reason: Error) => {
        if (alive) {
          setResult(null);
          setError(reason.message);
        }
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => { alive = false; };
  }, [type, targetId, debouncedQuery, needsInstance, aiEnabled, aiEntitled, useAI]);

  const projects = result?.items ?? [];
  const warnings = useMemo(() => {
    const list = (result?.warnings ?? []).filter((warning) => !warning.includes('CurseForge'));
    if (result && !result.curseForgeReady) {
      list.unshift(t('resources.cfMissing'));
    }
    return list;
  }, [result, t]);
  const gameVersion = useMemo(
    () => instances.find((instance) => instance.id === targetId)?.mcVersion ?? '',
    [instances, targetId],
  );

  const install = async (project: ResourceProject) => {
    const key = project.id;
    setInstalling(key);
    try {
      if (project.type === 'modpack') {
        const response = await api.installResourceModpack(project.source, project.projectId, '', mirrorId);
        await refreshInstances();
        if (response.instance.id) selectInstance(response.instance.id);
        pushToast('success', t('resources.modpackDone'), t('resources.modpackDetail', { name: response.instance.name }));
      } else {
        if (!targetId) {
          pushToast('error', t('resources.needsInstance'));
          return;
        }
        let response;
        try {
          response = await api.installResource(targetId, project.source, project.projectId, '', mirrorId, false);
        } catch (reason) {
          const message = (reason as Error).message;
          if (!message.includes('检测到模组冲突')) throw reason;
          if (!window.confirm(message)) return;
          response = await api.installResource(targetId, project.source, project.projectId, '', mirrorId, true);
        }
        const detail = t('resources.installDoneDetail', { name: response.versionName || project.title });
        const dependencyCount = response.dependencies?.length ?? 0;
        const dependencyNote = dependencyCount > 0
          ? ' · ' + t('resources.depsInstalled', { count: dependencyCount })
          : '';
        pushToast('success', t('resources.installDone'), detail + dependencyNote);
      }
    } catch (reason) {
      pushToast('error', t('resources.installFailed'), (reason as Error).message);
    } finally {
      setInstalling(null);
    }
  };

  return (
    <div className="page resources-page">
      <PageHeader
        kicker="RESOURCES"
        title={<>{t('resources.title')} <span className="title-accent">/</span> {t('resources.tail')}</>}
        description={t('resources.description')}
      />

      <section className="panel">
        <div className="resource-tabs">
          {tabs.map((item) => {
            const Icon = item.icon;
            return (
              <button
                key={item.key}
                type="button"
                className={cn('resource-tab', type === item.key && 'is-active')}
                onClick={() => setType(item.key)}
              >
                <Icon size={15} />
                {t(item.labelKey)}
              </button>
            );
          })}
        </div>

        <div className="toolbar resource-toolbar">
          <div className="resource-toolbar-main">
            {needsInstance && (
              <label className="resource-target">
                <span>{t('resources.target')}</span>
                <select
                  className="text-input"
                  value={targetId}
                  onChange={(event) => setTargetId(event.target.value)}
                >
                  {instances.map((instance) => (
                    <option key={instance.id} value={instance.id}>
                      {instance.name} · {instance.mcVersion}
                    </option>
                  ))}
                </select>
              </label>
            )}
            <label className="resource-mirror">
              <span>{t('resources.mirrorLabel')}</span>
              <select className="text-input" value={mirrorId} onChange={(event) => setMirrorId(event.target.value)}>
                {(mirrors.length > 0 ? mirrors : [{ id: 'official', name: t('settings.mirrorOfficial') } as ResourceMirror]).map((source) => (
                  <option key={source.id} value={source.id}>{source.name}</option>
                ))}
              </select>
            </label>
            <div className="search-field">
              <Search size={15} />
              <input
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder={t('resources.searchPlaceholder')}
                aria-label={t('resources.searchPlaceholder')}
              />
            </div>
          </div>
          <div className="resource-toolbar-actions">
            {aiEnabled ? (
              aiEntitled ? (
                <label className="ai-search-toggle">
                  <input type="checkbox" checked={useAI} onChange={(event) => setUseAI(event.target.checked)} />
                  <Sparkles size={13} />
                  <span>{t('resources.aiSearch')}</span>
                </label>
              ) : (
                <button type="button" className="text-button ai-enable" onClick={openLicense}>
                  <LockKeyhole size={13} /> {t('resources.aiLocked')}
                </button>
              )
            ) : (
              <button type="button" className="text-button ai-enable" onClick={() => navigate('settings')}>
                <Sparkles size={13} /> {t('resources.aiEnable')}
              </button>
            )}
            {needsInstance && (
              <button
                type="button"
                className="btn btn-ghost btn-sm"
                disabled={!targetId}
                onClick={() => setUpdatesTarget(instances.find((instance) => instance.id === targetId) ?? null)}
              >
                <RefreshCw size={14} /> {t('modUpdates.check')}
              </button>
            )}
            <span className="toolbar-count">{result ? `${result.total}` : ''}</span>
          </div>
        </div>

        {needsInstance && !targetId ? (
          <EmptyState
            compact
            icon={<Boxes size={24} />}
            title={t('resources.noInstance')}
            body={t('resources.noInstanceBody')}
            action={<button type="button" className="btn btn-primary btn-sm" onClick={() => navigate('versions')}>{t('home.addVersion')}</button>}
          />
        ) : (
          <>
            {result?.aiUsed && (
              <div className="ai-search-note">
                <Sparkles size={13} />
                <span>{t('resources.aiUsed', { query: result.queryUsed })}</span>
              </div>
            )}
            {result && !result.aiUsed && (result.queryAlternatives?.length ?? 0) > 1 && (
              <div className="ai-search-note">
                <Search size={13} />
                <span>{t('resources.queryExpanded', {
                  query: (result.queryAlternatives ?? []).slice(0, 2).join(' / '),
                })}</span>
              </div>
            )}
            {result?.aiError && (
              <div className="inline-banner banner-warning">
                <TriangleAlert size={16} />
                <div>
                  <strong>{t('resources.aiFailed')}</strong>
                  <span>{result.aiError}</span>
                </div>
              </div>
            )}
            {result?.cjkQuery && !result.aiUsed && (
              <div className="inline-banner banner-warning">
                <TriangleAlert size={16} />
                <div>
                  <strong>{t('resources.aiHintTitle')}</strong>
                  <span>{t('resources.aiHint')}</span>
                </div>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => navigate('settings')}>
                  {t('resources.aiOpenSettings')}
                </button>
              </div>
            )}
            {warnings.length > 0 && (
              <div className="inline-banner banner-warning">
                <TriangleAlert size={16} />
                <div>
                  <strong>{t('settings.curseForgeTitle')}</strong>
                  <span>{warnings.join(' · ')}</span>
                </div>
              </div>
            )}
            {error && (
              <div className="inline-banner banner-error">
                <TriangleAlert size={16} />
                <div><strong>{t('resources.installFailed')}</strong><span>{error}</span></div>
              </div>
            )}

            {loading ? (
              <div className="resource-grid">
                {Array.from({ length: 6 }).map((_, index) => (
                  <div className="resource-card resource-skeleton" key={index}>
                    <span className="skeleton skeleton-icon" />
                    <span className="skeleton skeleton-line wide" />
                    <span className="skeleton skeleton-line" />
                  </div>
                ))}
              </div>
            ) : projects.length === 0 ? (
              <EmptyState
                compact
                icon={<Search size={24} />}
                title={t('resources.emptyTitle')}
                body={t('resources.emptyBody')}
              />
            ) : (
              <div className="resource-grid">
                {projects.map((project) => (
                  <article className="resource-card" key={project.id}>
                    <div className="resource-card-head">
                      <div className="resource-icon">
                        <span>{initials(project.title)}</span>
                        {project.iconUrl && (
                          <img
                            src={project.iconUrl}
                            alt=""
                            loading="lazy"
                            onError={(event) => { event.currentTarget.style.display = 'none'; }}
                          />
                        )}
                      </div>
                      <div className="resource-title">
                        <strong>{project.title}</strong>
                        <span>{t('resources.by', { author: project.author || project.source })}</span>
                      </div>
                      <span className="resource-source">{project.source === 'modrinth' ? 'Modrinth' : 'CurseForge'}</span>
                    </div>
                    <p className="resource-description">{project.description}</p>
                    <div className="resource-foot">
                      <span className="resource-downloads">
                        {formatCompactNumber(project.downloads, locale)}
                      </span>
                      {gameVersion && project.type !== 'modpack' && (
                        <span className="resource-game">{gameVersion}</span>
                      )}
                      <button
                        type="button"
                        className="btn btn-primary btn-sm"
                        disabled={installing === project.id}
                        onClick={() => void install(project)}
                      >
                        {installing === project.id ? <LoaderCircle size={13} className="spin" /> : <PackageOpen size={13} />}
                        {installing === project.id
                          ? t('resources.installing')
                          : project.type === 'modpack'
                            ? t('resources.installModpack')
                            : t('resources.install')}
                      </button>
                    </div>
                  </article>
                ))}
              </div>
            )}
          </>
        )}
      </section>

      {updatesTarget && (
        <ModUpdatesModal
          instance={updatesTarget}
          onClose={() => setUpdatesTarget(null)}
          onUpdated={refreshInstances}
        />
      )}
    </div>
  );
}
