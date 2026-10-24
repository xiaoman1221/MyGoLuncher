import { useEffect, useRef, useState } from 'react';
import { FolderOpen, Square, TerminalSquare, X } from 'lucide-react';
import type { InstanceView } from '../lib/types';
import { useApp } from '../lib/app-context';
import { api } from '../lib/api';
import { onEvent } from '../lib/events';
import { loaderLabel } from '../lib/format';

interface ConsoleDrawerProps {
  instance: InstanceView;
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

export function ConsoleDrawer({ instance, onClose, onStop, onOpenFolder }: ConsoleDrawerProps) {
  const { t } = useApp();
  const scrollRef = useRef<HTMLDivElement>(null);
  const [logs, setLogs] = useState<string[]>([]);
  const pending = useRef<string[]>([]);
  const received = useRef<string[]>([]);
  const flushTimer = useRef<number | null>(null);

  // Logs are owned by this drawer instead of the app shell. High-frequency
  // launch:log events therefore no longer re-render every page, and updates
  // are batched to one paint every 80 ms.
  useEffect(() => {
    let alive = true;
    const instanceId = instance.id;
    setLogs([]);
    pending.current = [];
    received.current = [];
    const flush = () => {
      flushTimer.current = null;
      const next = pending.current;
      pending.current = [];
      if (next.length > 0) {
        setLogs((current) => [...current.slice(-600), ...next].slice(-600));
      }
    };
    const dispose = onEvent<{ instanceId: string; line: string }>('launch:log', (payload) => {
      if (payload.instanceId !== instanceId) return;
      received.current.push(payload.line);
      if (received.current.length > 600) received.current = received.current.slice(-600);
      pending.current.push(payload.line);
      if (flushTimer.current == null) flushTimer.current = window.setTimeout(flush, 80);
    });
    api.getInstanceLog(instanceId)
      .then((initial) => {
        if (!alive) return;
        const base = initial.slice(-600);
        const counts = new Map<string, number>();
        for (const line of base) counts.set(line, (counts.get(line) ?? 0) + 1);
        const extras = received.current.filter((line) => {
          const count = counts.get(line) ?? 0;
          if (count > 0) {
            counts.set(line, count - 1);
            return false;
          }
          return true;
        });
        const merged = [...base, ...extras].slice(-600);
        setLogs(merged);
        received.current = merged;
        pending.current = [];
      })
      .catch(() => {});
    return () => {
      alive = false;
      dispose();
      if (flushTimer.current != null) window.clearTimeout(flushTimer.current);
      flushTimer.current = null;
      pending.current = [];
    };
  }, [instance.id]);

  useEffect(() => {
    const element = scrollRef.current;
    if (element) element.scrollTop = element.scrollHeight;
  }, [logs]);

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.isComposing || event.keyCode === 229) return;
      if (event.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [onClose]);

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
