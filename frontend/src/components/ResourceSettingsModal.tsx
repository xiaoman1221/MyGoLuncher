import { useEffect, useState, type CSSProperties } from 'react';
import { Coffee, FolderOpen, Save } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';
import { Modal } from './Modal';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];

export function ResourceSettingsModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const {
    instances, bootstrap, refreshInstances, pushToast, openEdit, t,
  } = useApp();
  const instance = instances.find((item) => item.id === id);
  const [autoMemory, setAutoMemory] = useState(false);
  const [memory, setMemory] = useState(4096);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !instance) return;
    const auto = instance.autoMemory || instance.memoryMB === 0;
    setAutoMemory(auto);
    setMemory(auto ? (bootstrap?.autoMemoryMB ?? 4096) : (instance.memoryMB || 4096));
    setSaving(false);
  }, [open, instance, bootstrap?.autoMemoryMB]);

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

  const folders = [
    { dir: 'mods', label: t('servers.dir.mods'), count: instance.modCount },
    { dir: 'resourcepacks', label: t('resources.type.resourcepack'), count: instance.resourcePackCount },
    { dir: 'shaderpacks', label: t('resources.type.shader'), count: instance.shaderCount },
  ];

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('resourcesSettings.title')}
      eyebrow={`${instance.mcVersion} · ${loaderLabel(instance.loader, t('loader.vanilla'))}`}
      description={t('resourcesSettings.description')}
      size="md"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
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
              <button
                key={folder.dir}
                type="button"
                className="resource-folder-card"
                onClick={() => void openFolder(folder.dir)}
              >
                <span className="resource-folder-card-head">
                  <FolderOpen size={15} />
                  <span>{folder.label}</span>
                </span>
                <strong>{folder.count}</strong>
                <small>{t('resourcesSettings.openFolder')}</small>
              </button>
            ))}
          </div>
        </div>

        <div className="field">
          <div className="field-label-row">
            <span className="field-label">{t('edit.memory')}</span>
            <span className="memory-value">
              {autoMemory
                ? t('settings.autoMemoryValue', { gb: ((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1) })
                : `${(memory / 1024).toFixed(memory % 1024 ? 1 : 0)} GB`}
            </span>
          </div>
          <label className="toggle-row">
            <span>
              <strong>{t('edit.autoMemory')}</strong>
              <small>{t('edit.autoMemoryHint')}</small>
            </span>
            <input
              type="checkbox"
              className="switch"
              checked={autoMemory}
              onChange={(event) => {
                setAutoMemory(event.target.checked);
                if (!event.target.checked) setMemory(bootstrap?.autoMemoryMB ?? 4096);
              }}
            />
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
                style={{ '--range-progress': `${((memory - 1024) / (16384 - 1024)) * 100}%` } as CSSProperties}
              />
              <div className="memory-presets">
                {memorySteps.map((value) => (
                  <button
                    key={value}
                    type="button"
                    className={`chip chip-button ${memory === value ? 'is-active' : ''}`}
                    onClick={() => setMemory(value)}
                  >
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
            <button
              type="button"
              className="btn btn-ghost btn-sm"
              onClick={() => { onClose(); openEdit(instance.id); }}
            >
              {t('resourcesSettings.editJava')}
            </button>
          </div>
        </div>
      </div>
    </Modal>
  );
}
