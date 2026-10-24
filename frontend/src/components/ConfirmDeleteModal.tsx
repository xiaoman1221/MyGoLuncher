import { useEffect, useState } from 'react';
import { Trash2, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { Modal } from './Modal';

export function ConfirmDeleteModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const { instances, refreshInstances, pushToast, selectInstance, selectedId } = useApp();
  const instance = instances.find((item) => item.id === id);
  const [deleteFiles, setDeleteFiles] = useState(false);
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (open) {
      setDeleteFiles(false);
      setDeleting(false);
    }
  }, [open]);

  if (!instance) return null;

  const remove = async () => {
    setDeleting(true);
    try {
      await api.deleteInstance(instance.id, deleteFiles);
      await refreshInstances();
      if (selectedId === instance.id) {
        const next = instances.find((item) => item.id !== instance.id);
        if (next) selectInstance(next.id);
      }
      pushToast('success', deleteFiles ? '版本与文件已删除' : '版本已从启动器移除');
      onClose();
    } catch (error) {
      pushToast('error', '删除失败', (error as Error).message);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="删除版本？"
      eyebrow="DELETE"
      size="sm"
      footer={
        <div className="modal-actions modal-actions-end">
          <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
          <button type="button" className="btn btn-danger" onClick={() => void remove()} disabled={deleting}>
            <Trash2 size={15} /> {deleting ? '正在删除…' : '确认删除'}
          </button>
        </div>
      }
    >
      <div className="danger-callout">
        <div className="danger-icon"><TriangleAlert size={20} /></div>
        <div>
          <strong>{instance.name}</strong>
          <p>{instance.mcVersion} · 从启动器列表中移除这个版本。</p>
        </div>
      </div>
      <label className="checkbox-row">
        <input
          type="checkbox"
          checked={deleteFiles}
          onChange={(event) => setDeleteFiles(event.target.checked)}
        />
        <span>
          <strong>同时删除游戏目录</strong>
          <small>存档、模组与配置将永久删除。</small>
        </span>
      </label>
    </Modal>
  );
}
