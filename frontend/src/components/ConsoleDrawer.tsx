import { useEffect, useRef, useState } from 'react';
import { Check, Copy, FolderOpen, Square, TerminalSquare, X } from 'lucide-react';
import type { InstanceView } from '../lib/types';
import { useApp } from '../lib/app-context';
import { loaderLabel } from '../lib/format';

interface ConsoleDrawerProps {
  instance: InstanceView;
  logs: string[];
  onClose: () => void;
  onStop: () => void;
  onOpenFolder: () => void;
}

function lineTone(line: string): string {
  const upper = line.toUpperCase();
  if (upper.includes('ERROR') || upper.includes('FATAL') || upper.includes('EXCEPTION')) return 'log-error';
  if (upper.includes('WARN')) return 'log-warn';
  if (upper.includes('启动器') || upper.includes('MYGOLUNCHER')) return 'log-launcher';
  return '';
}

export function ConsoleDrawer({ instance, logs, onClose, onStop, onOpenFolder }: ConsoleDrawerProps) {
  const { t } = useApp();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [logs]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(logs.join('\n'));
      setCopied(true);
      window.setTimeout(() => setCopied(false), 1500);
    } catch {
      setCopied(false);
    }
  };

  return (
    <aside className="console-drawer" aria-label={t('console.title')}>
      <header className="console-head">
        <div className="console-lights" aria-hidden="true">
          <span className="red" /><span className="amber" /><span className="green" />
        </div>
        <div className="console-title">
          <TerminalSquare size={16} />
          <span>{t('console.title')}</span>
        </div>
        <div className={`console-status ${instance.running ? 'is-live' : ''}`}>
          {instance.running ? <><span className="live-pulse" />{t('home.running')}</> : t('console.stopped')}
        </div>
        <button type="button" className="icon-button icon-button-sm" onClick={onClose} aria-label={t('console.close')}>
          <X size={16} />
        </button>
      </header>

      <div className="console-meta">
        <div>
          <strong>{instance.name}</strong>
          <span>{instance.mcVersion} · {loaderLabel(instance.loader, t('loader.vanilla'))} · Java {instance.requiredJava}</span>
        </div>
        <div className="console-actions">
          <button type="button" className="icon-button icon-button-sm" title={t('console.copyLogs')} onClick={() => void copy()}>
            {copied ? <Check size={15} /> : <Copy size={15} />}
          </button>
          <button type="button" className="icon-button icon-button-sm" title={t('console.openFolder')} onClick={onOpenFolder}>
            <FolderOpen size={15} />
          </button>
        </div>
      </div>

      <div className="console-body" ref={scrollRef}>
        {logs.length === 0 ? (
          <div className="console-empty">
            <TerminalSquare size={24} />
            <span>{t('console.waiting')}</span>
          </div>
        ) : (
          logs.map((line, index) => (
            <div className={`console-line ${lineTone(line)}`} key={`${index}-${line.slice(0, 12)}`}>
              <span className="console-prompt">›</span>
              <span>{line}</span>
            </div>
          ))
        )}
      </div>

      <footer className="console-foot">
        <span className="console-hint">{t('console.hint')}</span>
        {instance.running ? (
          <button type="button" className="btn btn-danger btn-sm" onClick={onStop}>
            <Square size={13} fill="currentColor" /> {t('console.endGame')}
          </button>
        ) : (
          <button type="button" className="btn btn-ghost btn-sm" onClick={onClose}>{t('common.close')}</button>
        )}
      </footer>
    </aside>
  );
}
