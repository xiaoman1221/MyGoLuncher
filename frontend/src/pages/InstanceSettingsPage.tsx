import { FolderOpen, Pencil, Plus, Trash2 } from 'lucide-react';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { cn, loaderLabel, loaderTone } from '../lib/format';

export function InstanceSettingsPage() {
  const {
    instances, settings, openCreate, openEdit, askDelete, openInstanceFolder, t,
  } = useApp();
  const isolated = settings?.versionIsolation !== false;

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
    </div>
  );
}
