import { useEffect, useState, type CSSProperties } from 'react';
import { Coffee, FolderOpen, Save, Trash2 } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';
import { Modal } from './Modal';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];

export function EditInstanceModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const {
    instances, settings, bootstrap, refreshInstances, pushToast, openInstanceFolder, askDelete, t,
  } = useApp();
  const instance = instances.find((item) => item.id === id);
  const [name, setName] = useState('');
  const [memory, setMemory] = useState(4096);
  const [autoMemory, setAutoMemory] = useState(false);
  const [javaPath, setJavaPath] = useState('');
  const [saving, setSaving] = useState(false);

  // Only initialize when the dialog opens or switches instances. Depending on
  // the whole instance object would reset fields whenever a background event
  // refreshes the instance list.
  useEffect(() => {
    if (!open || !instance) return;
    setName(instance.name);
    const auto = instance.autoMemory || instance.memoryMB === 0;
    setAutoMemory(auto);
    setMemory(auto ? (bootstrap?.autoMemoryMB ?? 4096) : (instance.memoryMB || 4096));
    setJavaPath(instance.javaPath);
    setSaving(false);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, instance?.id]);

  if (!instance) return null;

  const save = async () => {
    if (!name.trim()) {
      pushToast('error', t('edit.nameRequired'));
      return;
    }
    setSaving(true);
    try {
      await api.updateInstance({ id: instance.id, name: name.trim(), memoryMB: autoMemory ? 0 : memory, javaPath: javaPath.trim() });
      await refreshInstances();
      pushToast('success', t('edit.saved'));
      onClose();
    } catch (error) {
      pushToast('error', t('edit.saveFailed'), (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('edit.title')}
      eyebrow={`${instance.mcVersion} · ${loaderLabel(instance.loader, t('loader.vanilla'))}`}
      size="md"
      footer={
        <>
          <button
            type="button"
            className="btn btn-danger-ghost"
            onClick={() => { onClose(); askDelete(instance.id); }}
          >
            <Trash2 size={15} /> {t('common.deleteVersion')}
          </button>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
            <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving}>
              <Save size={15} /> {saving ? t('common.saving') : t('common.save')}
            </button>
          </div>
        </>
      }
    >
      <div className="form-grid">
        <label className="field field-span-2">
          <span className="field-label">{t('edit.name')}</span>
          <input className="text-input" value={name} onChange={(event) => setName(event.target.value)} maxLength={60} />
        </label>

        <div className="field field-span-2">
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
        </div>

        <label className="field field-span-2">
          <span className="field-label"><Coffee size={14} /> {t('edit.javaPath')} <small>{t('edit.javaPathHint')}</small></span>
          <input
            className="text-input mono"
            value={javaPath}
            onChange={(event) => setJavaPath(event.target.value)}
            placeholder={settings?.defaultJavaPath || t('edit.javaPathHint')}
          />
        </label>

        <div className="form-actions-inline field-span-2">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void openInstanceFolder(instance.id)}>
            <FolderOpen size={15} /> {t('edit.openFolder')}
          </button>
        </div>
      </div>
    </Modal>
  );
}
