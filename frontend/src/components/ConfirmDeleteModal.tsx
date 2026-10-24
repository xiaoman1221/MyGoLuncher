import { useEffect, useState } from 'react';
import { Trash2, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { Modal } from './Modal';

export function ConfirmDeleteModal({ id, open, onClose }: { id: string | null; open: boolean; onClose: () => void }) {
  const { instances, settings, refreshInstances, pushToast, selectInstance, selectedId, t } = useApp();
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
      pushToast('success', deleteFiles ? t('delete.successFiles') : t('delete.successKeep'));
      onClose();
    } catch (error) {
      pushToast('error', t('delete.failed'), (error as Error).message);
    } finally {
      setDeleting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('delete.title')}
      eyebrow="DELETE"
      size="sm"
      footer={
        <div className="modal-actions modal-actions-end">
          <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
          <button type="button" className="btn btn-danger" onClick={() => void remove()} disabled={deleting}>
            <Trash2 size={15} /> {deleting ? t('delete.deleting') : t('delete.confirm')}
          </button>
        </div>
      }
    >
      <div className="danger-callout">
        <div className="danger-icon"><TriangleAlert size={20} /></div>
        <div>
          <strong>{instance.name}</strong>
          <p>{t('delete.body', { version: instance.mcVersion })}</p>
        </div>
      </div>
      {settings?.versionIsolation !== false ? (
        <label className="checkbox-row">
          <input
            type="checkbox"
            checked={deleteFiles}
            onChange={(event) => setDeleteFiles(event.target.checked)}
          />
          <span>
            <strong>{t('delete.deleteFiles')}</strong>
            <small>{t('delete.deleteFilesHint')}</small>
          </span>
        </label>
      ) : (
        <p className="modal-note">{t('delete.sharedWarning')}</p>
      )}
    </Modal>
  );
}
