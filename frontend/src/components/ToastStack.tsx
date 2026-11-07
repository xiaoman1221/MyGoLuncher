import { CheckCircle2, Info, TriangleAlert, X } from 'lucide-react';
import type { Toast } from '../lib/app-context';
import { useApp } from '../lib/app-context';

interface ToastStackProps {
  toasts: Toast[];
  onDismiss: (id: number) => void;
  withConsole?: boolean;
}

const icons = {
  success: CheckCircle2,
  error: TriangleAlert,
  info: Info,
};

export function ToastStack({ toasts, onDismiss, withConsole }: ToastStackProps) {
  const { t } = useApp();
  if (toasts.length === 0) return null;
  return (
    <div className={`toast-stack ${withConsole ? 'with-console' : ''}`} aria-live="polite">
      {toasts.map((toast) => {
        const Icon = icons[toast.kind];
        return (
          <div key={toast.id} className={`toast toast-${toast.kind}`}>
            <div className="toast-icon"><Icon size={18} /></div>
            <div className="toast-copy">
              <strong>{toast.title}</strong>
              {toast.message && <span>{toast.message}</span>}
            </div>
            {toast.action && (
              <button
                type="button"
                className="toast-action"
                onClick={() => { toast.action?.onClick(); onDismiss(toast.id); }}
              >
                {toast.action.label}
              </button>
            )}
            <button type="button" className="toast-close" onClick={() => onDismiss(toast.id)} aria-label={t('common.close')}>
              <X size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
