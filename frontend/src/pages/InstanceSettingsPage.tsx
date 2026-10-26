import { useState } from 'react';
import { FolderOpen, LoaderCircle, Pencil, Plus, RefreshCw, Settings2, Trash2, Wrench } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { ModUpdatesModal } from '../components/ModUpdatesModal';
import { ResourceSettingsModal } from '../components/ResourceSettingsModal';
import { cn, loaderLabel, loaderTone } from '../lib/format';
import { api } from '../lib/api';

export function InstanceSettingsPage() {
  const {
    instances, settings, openCreate, openEdit, askDelete, openInstanceFolder, refreshInstances,
    hasFeature, openLicense, pushToast, t,
  } = useApp();
  const isolated = settings?.versionIsolation !== false;
  const [updatesId, setUpdatesId] = useState<string | null>(null);
  const [resourcesId, setResourcesId] = useState<string | null>(null);
  const [repairingId, setRepairingId] = useState<string | null>(null);
  const updateInstance = instances.find((instance) => instance.id === updatesId) ?? null;

  const repairVersion = async (instanceId: string) => {
    if (!hasFeature('versions')) {
      openLicense();
      return;
    }
    setRepairingId(instanceId);
    try {
      await api.repairVersion(instanceId);
      pushToast('success', t('instanceSettings.repairDone'));
      refreshInstances();
    } catch (reason) {
      pushToast('error', t('instanceSettings.repairFailed'), (reason as Error).message);
    } finally {
      setRepairingId(null);
    }
  };

  return (
    <div className="page instance-settings-page">
      <PageHeader
        kicker="VERSION SETTINGS"
        title={<>{t('instanceSettings.title')} <span className="title-accent">/</span> MyGoLuncher</>}
        description={t('instanceSettings.description')}
        actions={
          <button type="button" className="btn btn-primary" onClick={openCreate}>
            <Plus size={16} /> {t('instanceSettings.newVersion')}
          </button>
        }
      />

      <div className={cn('isolation-note', isolated ? 'is-on' : 'is-off')}>
        <span className="isolation-dot" />
        <span>{isolated ? t('instanceSettings.isolationOn') : t('instanceSettings.isolationOff')}</span>
      </div>

      {instances.length === 0 ? (
        <EmptyState
          icon={<Pencil size={24} />}
          title={t('instanceSettings.emptyTitle')}
          body={t('instanceSettings.emptyBody')}
          action={<button type="button" className="btn btn-primary" onClick={openCreate}><Plus size={16} /> {t('instanceSettings.newVersion')}</button>}
        />
      ) : (
        <div className="version-settings-list">
          {instances.map((instance) => (
            <div className={cn('version-settings-row', `tone-${loaderTone(instance.loader)}`)} key={instance.id}>
              <div className="version-settings-mark">{instance.mcVersion}</div>
              <div className="version-settings-copy">
                <div className="version-settings-title">
                  <strong>{instance.name}</strong>
                  {instance.running && <span className="running-pill"><span className="live-pulse" />{t('home.running')}</span>}
                </div>
                <span className="version-settings-meta">
                  {t('instanceSettings.versionLine', {
                    version: instance.mcVersion,
                    loader: loaderLabel(instance.loader, t('loader.vanilla')),
                    java: instance.requiredJava,
                  })}
                </span>
                <span className="version-settings-meta">
                  {t('instanceSettings.memory', { value: `${((instance.effectiveMemoryMB || 0) / 1024).toFixed(0)} GB` })}
                </span>
                <code className="version-settings-dir" title={instance.gameDir}>{instance.gameDir}</code>
              </div>
              <div className="version-settings-actions">
                {(instance.loader || 'vanilla').toLowerCase() !== 'vanilla' && (
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm version-resource-button"
                    title={t('instanceSettings.resourcesHint')}
                    onClick={() => setResourcesId(instance.id)}
                  >
                    <Settings2 size={14} /> {t('instanceSettings.resources')}
                  </button>
                )}
                <button
                  type="button"
                  className="btn btn-ghost btn-sm"
                  disabled={instance.running || repairingId === instance.id}
                  title={instance.running ? t('instanceSettings.repairRunning') : t('instanceSettings.repairHint')}
                  onClick={() => void repairVersion(instance.id)}
                >
                  {repairingId === instance.id
                    ? <LoaderCircle size={14} className="spin" />
                    : <Wrench size={14} />}
                  {repairingId === instance.id ? t('instanceSettings.repairing') : t('instanceSettings.repair')}
                </button>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => setUpdatesId(instance.id)}>
                  <RefreshCw size={14} /> {t('modUpdates.check')}
                </button>
                <button type="button" className="btn btn-ghost btn-sm" onClick={() => openEdit(instance.id)}>
                  <Pencil size={14} /> {t('instanceSettings.edit')}
                </button>
                <button type="button" className="icon-button icon-button-sm" title={t('common.openGameFolder')} onClick={() => void openInstanceFolder(instance.id)}>
                  <FolderOpen size={15} />
                </button>
                <button type="button" className="icon-button icon-button-sm version-settings-delete" title={t('common.deleteVersion')} onClick={() => askDelete(instance.id)}>
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
          ))}
        </div>
      )}

      {updateInstance && (
        <ModUpdatesModal
          instance={updateInstance}
          onClose={() => setUpdatesId(null)}
          onUpdated={refreshInstances}
        />
      )}

      <ResourceSettingsModal
        id={resourcesId}
        open={Boolean(resourcesId)}
        onClose={() => setResourcesId(null)}
      />
    </div>
  );
}
