import { useEffect, useState, type CSSProperties } from 'react';
import { Coffee, FolderOpen, LoaderCircle, Save } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';
import { Modal } from './Modal';
import type { InstanceResource, InstanceResourceKind, InstanceResources } from '../lib/types';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];
const emptyResources: InstanceResources = { mods: [], resourcePacks: [], shaders: [] };

function formatBytes(value: number): string {
  if (value >= 1024 * 1024) return (value / (1024 * 1024)).toFixed(1) + ' MB';
  if (value >= 1024) return (value / 1024).toFixed(0) + ' KB';
  return value + ' B';
}

export function ResourceSettingsModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const { instances, bootstrap, refreshInstances, pushToast, openEdit, t } = useApp();
  const instance = instances.find((item) => item.id === id);
  const [autoMemory, setAutoMemory] = useState(false);
  const [memory, setMemory] = useState(4096);
  const [saving, setSaving] = useState(false);
  const [resources, setResources] = useState<InstanceResources | null>(null);
  const [resourcesLoading, setResourcesLoading] = useState(false);
  const [resourceBusy, setResourceBusy] = useState('');

  useEffect(() => {
    if (!open || !instance) return;
    const auto = instance.autoMemory || instance.memoryMB === 0;
    setAutoMemory(auto);
    setMemory(auto ? (bootstrap?.autoMemoryMB ?? 4096) : (instance.memoryMB || 4096));
    setSaving(false);
  }, [open, instance, bootstrap?.autoMemoryMB]);

  useEffect(() => {
    if (!open || !instance) return;
    let active = true;
    setResourcesLoading(true);
    setResources(null);
    api.getInstanceResources(instance.id)
      .then((next) => { if (active) setResources(next); })
      .catch((error: Error) => {
        if (active) {
          setResources(emptyResources);
          pushToast('error', t('resourcesSettings.loadFailed'), error.message);
        }
      })
      .finally(() => { if (active) setResourcesLoading(false); });
    return () => { active = false; };
  }, [open, instance?.id, pushToast, t]);

  if (!instance) return null;

  const openFolder = async (folder: string) => {
    try {
      await api.openInstanceSubfolder(instance.id, folder);
    } catch (error) {
      pushToast('error', t('resourcesSettings.openFailed'), (error as Error).message);
    }
  };

  const save = async () => {
    setSaving(true);
    try {
      await api.updateInstance({
        id: instance.id,
        name: instance.name,
        memoryMB: autoMemory ? 0 : memory,
        javaPath: instance.javaPath,
      });
      await refreshInstances();
      pushToast('success', t('edit.saved'));
      onClose();
    } catch (error) {
      pushToast('error', t('edit.saveFailed'), (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  const toggleResource = async (item: InstanceResource) => {
    const key = item.kind + ':' + item.fileName;
    setResourceBusy(key);
    try {
      const next = await api.setInstanceResourceEnabled(instance.id, item.kind, item.fileName, !item.enabled);
      setResources(next);
      await refreshInstances();
      pushToast(
        'success',
        item.enabled ? t('resourcesSettings.disabledNow') : t('resourcesSettings.enabledNow'),
        item.name,
      );
    } catch (error) {
      pushToast('error', t('resourcesSettings.toggleFailed'), (error as Error).message);
    } finally {
      setResourceBusy('');
    }
  };

  const enabledCount = (items: InstanceResource[]) => items.filter((item) => item.enabled).length;
  const folders = [
    { dir: 'mods', label: t('servers.dir.mods'), count: resources ? enabledCount(resources.mods) : instance.modCount },
    { dir: 'resourcepacks', label: t('resources.type.resourcepack'), count: resources ? enabledCount(resources.resourcePacks) : instance.resourcePackCount },
    { dir: 'shaderpacks', label: t('resources.type.shader'), count: resources ? enabledCount(resources.shaders) : instance.shaderCount },
  ];
  const groups: Array<{ kind: InstanceResourceKind; title: string; items: InstanceResource[] }> = [
    { kind: 'mods', title: t('servers.dir.mods'), items: resources?.mods ?? [] },
    { kind: 'resourcepacks', title: t('resources.type.resourcepack'), items: resources?.resourcePacks ?? [] },
    { kind: 'shaderpacks', title: t('resources.type.shader'), items: resources?.shaders ?? [] },
  ];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('resourcesSettings.title')}
      eyebrow={instance.mcVersion + ' · ' + loaderLabel(instance.loader, t('loader.vanilla'))}
      description={t('resourcesSettings.description')}
      size="xl"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.close')}</button>
          <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving}>
            <Save size={15} /> {saving ? t('common.saving') : t('common.save')}
          </button>
        </>
      }
    >
      <div className="settings-fields">
        <div className="field">
          <span className="field-label">{t('resourcesSettings.overview')}</span>
          <div className="resource-folder-grid">
            {folders.map((folder) => (
              <button key={folder.dir} type="button" className="resource-folder-card" onClick={() => void openFolder(folder.dir)}>
                <span className="resource-folder-card-head"><FolderOpen size={15} /><span>{folder.label}</span></span>
                <strong>{folder.count}</strong>
                <small>{t('resourcesSettings.openFolder')}</small>
              </button>
            ))}
          </div>
        </div>

        <div className="field">
          <div className="field-label-row">
            <span className="field-label">{t('resourcesSettings.installed')}</span>
            {resources && <span className="memory-value">{t('resourcesSettings.total', { count: resources.mods.length + resources.resourcePacks.length + resources.shaders.length })}</span>}
          </div>
          <span className="field-hint">{t('resourcesSettings.installedHint')}</span>
          {resourcesLoading ? (
            <div className="resource-manager-loading"><LoaderCircle size={16} className="spin" /> {t('resourcesSettings.loading')}</div>
          ) : (
            <div className="resource-manager">
              {groups.map((group) => (
                <div className="resource-group" key={group.kind}>
                  <div className="resource-group-head">
                    <span>{group.title}</span>
                    <small>{enabledCount(group.items)} / {group.items.length}</small>
                  </div>
                  {group.items.length === 0 ? (
                    <div className="resource-manager-empty">{t('resourcesSettings.empty')}</div>
                  ) : (
                    <div className="resource-list">
                      {group.items.map((item) => {
                        const key = item.kind + ':' + item.fileName;
                        return (
                          <div className={item.enabled ? 'resource-item' : 'resource-item is-disabled'} key={item.fileName}>
                            <span className="resource-item-name">
                              <strong title={item.name}>{item.name}</strong>
                              <small>{item.directory ? t('resourcesSettings.folder') + ' · ' : ''}{formatBytes(item.size)}</small>
                            </span>
                            <span className="resource-item-state">{item.enabled ? t('common.enabled') : t('common.disabled')}</span>
                            <label className="resource-item-switch">
                              <input
                                type="checkbox"
                                className="switch"
                                checked={item.enabled}
                                disabled={resourceBusy === key}
                                onChange={() => void toggleResource(item)}
                                aria-label={item.name}
                              />
                            </label>
                          </div>
                        );
                      })}
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>

        <div className="field">
          <div className="field-label-row">
            <span className="field-label">{t('edit.memory')}</span>
            <span className="memory-value">{autoMemory ? t('settings.autoMemoryValue', { gb: ((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1) }) : (memory / 1024).toFixed(memory % 1024 ? 1 : 0) + ' GB'}</span>
          </div>
          <label className="toggle-row">
            <span><strong>{t('edit.autoMemory')}</strong><small>{t('edit.autoMemoryHint')}</small></span>
            <input type="checkbox" className="switch" checked={autoMemory} onChange={(event) => {
              setAutoMemory(event.target.checked);
              if (!event.target.checked) setMemory(bootstrap?.autoMemoryMB ?? 4096);
            }} />
          </label>
          {!autoMemory && (
            <>
              <input
                className="range"
                type="range"
                min={1024}
                max={16384}
                step={512}
                value={memory}
                onChange={(event) => setMemory(Number(event.target.value))}
                style={{ '--range-progress': ((memory - 1024) / (16384 - 1024)) * 100 + '%' } as CSSProperties}
              />
              <div className="memory-presets">
                {memorySteps.map((value) => (
                  <button key={value} type="button" className={'chip chip-button ' + (memory === value ? 'is-active' : '')} onClick={() => setMemory(value)}>
                    {value / 1024} GB
                  </button>
                ))}
              </div>
            </>
          )}
          <span className="field-hint">{t('resourcesSettings.memoryHint')}</span>
        </div>

        <div className="field">
          <span className="field-label"><Coffee size={14} /> {t('edit.javaPath')}</span>
          <div className="resource-java-row">
            <code>{instance.javaPath || t('edit.javaPathHint')}</code>
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => { onClose(); openEdit(instance.id); }}>{t('resourcesSettings.editJava')}</button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
