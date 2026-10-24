import { useEffect, useId, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  eyebrow?: string;
  description?: string;
  children: ReactNode;
  footer?: ReactNode;
  size?: 'sm' | 'md' | 'lg' | 'xl';
  closeable?: boolean;
}

export function Modal({
  open,
  onClose,
  title,
  eyebrow,
  description,
  children,
  footer,
  size = 'md',
  closeable = true,
}: ModalProps) {
  const titleId = useId();
  const cardRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape' && closeable) onClose();
    };
    document.addEventListener('keydown', onKeyDown);
    const timer = window.setTimeout(() => {
      const focusable = cardRef.current?.querySelector<HTMLElement>(
        'input:not([disabled]), select:not([disabled]), textarea:not([disabled]), button:not([disabled])',
      );
      focusable?.focus();
    }, 60);
    return () => {
      document.body.style.overflow = previousOverflow;
      document.removeEventListener('keydown', onKeyDown);
      window.clearTimeout(timer);
    };
  }, [open, onClose, closeable]);

  if (!open) return null;

  return createPortal(
    <div className="modal-layer" onMouseDown={(event) => {
      if (event.target === event.currentTarget && closeable) onClose();
    }}>
      <div
        ref={cardRef}
        className={`modal-card modal-${size}`}
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <div className="modal-glow" aria-hidden="true" />
        <header className="modal-header">
          <div>
            {eyebrow && <div className="eyebrow"><span className="eyebrow-dash" />{eyebrow}</div>}
            <h2 id={titleId}>{title}</h2>
            {description && <p>{description}</p>}
          </div>
          {closeable && (
            <button type="button" className="icon-button modal-close" onClick={onClose} aria-label="关闭">
              <X size={18} />
            </button>
          )}
        </header>
        <div className="modal-body">{children}</div>
        {footer && <footer className="modal-footer">{footer}</footer>}
      </div>
    </div>,
    document.body,
  );
}
