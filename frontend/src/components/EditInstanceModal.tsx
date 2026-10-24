import { useEffect, useState, type CSSProperties } from 'react';
import { Coffee, FolderOpen, Save, Trash2 } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';
import { Modal } from './Modal';

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];

export function EditInstanceModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const {
    instances, settings, bootstrap, refreshInstances, pushToast, openInstanceFolder, askDelete,
  } = useApp();
  const instance = instances.find((item) => item.id === id);
  const [name, setName] = useState('');
  const [memory, setMemory] = useState(4096);
  const [autoMemory, setAutoMemory] = useState(false);
  const [javaPath, setJavaPath] = useState('');
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    if (!open || !instance) return;
    setName(instance.name);
    const auto = instance.autoMemory || instance.memoryMB === 0;
    setAutoMemory(auto);
    setMemory(auto ? (bootstrap?.autoMemoryMB ?? 4096) : (instance.memoryMB || 4096));
    setJavaPath(instance.javaPath);
    setSaving(false);
  }, [open, instance, bootstrap?.autoMemoryMB]);

  if (!instance) return null;

  const save = async () => {
    if (!name.trim()) {
      pushToast('error', '版本名称不能为空');
      return;
    }
    setSaving(true);
    try {
      await api.updateInstance({ id: instance.id, name: name.trim(), memoryMB: autoMemory ? 0 : memory, javaPath: javaPath.trim() });
      await refreshInstances();
      pushToast('success', '版本设置已保存');
      onClose();
    } catch (error) {
      pushToast('error', '保存失败', (error as Error).message);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="版本设置"
      eyebrow={`${instance.mcVersion} · ${loaderLabel(instance.loader)}`}
      size="md"
      footer={
        <>
          <button
            type="button"
            className="btn btn-danger-ghost"
            onClick={() => { onClose(); askDelete(instance.id); }}
          >
            <Trash2 size={15} /> 删除版本
          </button>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
            <button type="button" className="btn btn-primary" onClick={() => void save()} disabled={saving}>
              <Save size={15} /> {saving ? '保存中…' : '保存'}
            </button>
          </div>
        </>
      }
    >
      <div className="form-grid">
        <label className="field field-span-2">
          <span className="field-label">版本名称</span>
          <input className="text-input" value={name} onChange={(event) => setName(event.target.value)} maxLength={60} />
        </label>

        <div className="field field-span-2">
          <div className="field-label-row">
            <span className="field-label">分配内存</span>
            <span className="memory-value">
              {autoMemory
                ? `自动 · 约 ${((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1)} GB`
                : `${(memory / 1024).toFixed(memory % 1024 ? 1 : 0)} GB`}
            </span>
          </div>
          <label className="toggle-row">
            <span>
              <strong>自动分配内存</strong>
              <small>根据系统内存自动计算推荐值</small>
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
          <span className="field-label"><Coffee size={14} /> Java 路径 <small>留空使用默认运行时</small></span>
          <input
            className="text-input mono"
            value={javaPath}
            onChange={(event) => setJavaPath(event.target.value)}
            placeholder={settings?.defaultJavaPath || `默认 Java ${instance.requiredJava}`}
          />
        </label>

        <div className="form-actions-inline field-span-2">
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => void openInstanceFolder(instance.id)}>
            <FolderOpen size={15} /> 打开版本目录
          </button>
        </div>
      </div>
    </Modal>
  );
}
