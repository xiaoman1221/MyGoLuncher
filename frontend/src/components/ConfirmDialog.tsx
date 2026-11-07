import { useApp } from '../lib/app-context';
import type { ConfirmOptions } from '../lib/app-context';
import { Modal } from './Modal';

export function ConfirmDialog({
  options,
  onResolve,
}: {
  options: ConfirmOptions | null;
  onResolve: (value: boolean) => void;
}) {
  const { t } = useApp();
  if (!options) return null;
  return (
    <Modal
      open
      onClose={() => onResolve(false)}
      title={options.title ?? t('common.confirm')}
      size="sm"
      footer={
        <div className="modal-actions modal-actions-end">
          <button type="button" className="btn btn-ghost" onClick={() => onResolve(false)}>
            {options.cancelLabel ?? t('common.cancel')}
          </button>
          <button
            type="button"
            className={options.danger ? 'btn btn-danger' : 'btn btn-primary'}
            onClick={() => onResolve(true)}
          >
            {options.confirmLabel ?? t('common.confirm')}
          </button>
        </div>
      }
    >
      <p className="confirm-message">{options.message}</p>
    </Modal>
  );
}
