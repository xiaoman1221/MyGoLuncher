import { useEffect, useId, useRef, type ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { X } from 'lucide-react';
import { useApp } from '../lib/app-context';

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
  overlayClassName?: string;
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
  overlayClassName = '',
}: ModalProps) {
  const { t } = useApp();
  const titleId = useId();
  const cardRef = useRef<HTMLDivElement>(null);
  const onCloseRef = useRef(onClose);
  const closeableRef = useRef(closeable);

  useEffect(() => {
    onCloseRef.current = onClose;
    closeableRef.current = closeable;
  }, [onClose, closeable]);

  useEffect(() => {
    if (!open) return;
    const previousOverflow = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKeyDown = (event: KeyboardEvent) => {
      // Do not close the dialog while an IME candidate window is active:
      // Escape is also used to cancel Chinese/Japanese/Korean composition.
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === 'Escape' && closeableRef.current) onCloseRef.current();
    };
    document.addEventListener('keydown', onKeyDown);
    const timer = window.setTimeout(() => {
      // Only focus the first field when focus is not already inside the modal.
      // Re-focusing on every parent render breaks typing and IME composition.
      const active = document.activeElement;
      if (active && cardRef.current?.contains(active)) return;
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
    // Intentionally only on open: onClose/closeable are read through refs so
    // parent re-renders do not steal focus from the field being typed in.
  }, [open]);

  if (!open) return null;

  return createPortal(
    <div className={`modal-layer ${overlayClassName}`.trim()} onMouseDown={(event) => {
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
            <button type="button" className="icon-button modal-close" onClick={onClose} aria-label={t('common.close')}>
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
