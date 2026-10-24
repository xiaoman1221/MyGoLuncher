import { CheckCircle2, Info, TriangleAlert, X } from 'lucide-react';
import type { Toast } from '../lib/app-context';

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
            <button type="button" className="toast-close" onClick={() => onDismiss(toast.id)} aria-label="关闭通知">
              <X size={14} />
            </button>
          </div>
        );
      })}
    </div>
  );
}
