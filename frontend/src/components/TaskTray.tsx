import { useEffect, useRef, useState } from 'react';
import {
  Ban,
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  FileText,
  LoaderCircle,
  TriangleAlert,
  X,
} from 'lucide-react';
import type { TaskLog, TaskLogEvent, TaskProgress } from '../lib/types';
import type { MessageKey } from '../lib/i18n';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import { api } from '../lib/api';

const stageKeys: Record<string, MessageKey> = {
  prepare: 'stage.prepare',
  game: 'stage.game',
  loader: 'stage.loader',
  java: 'stage.java',
  download: 'stage.download',
  assets: 'stage.assets',
  libraries: 'stage.libraries',
  import: 'stage.import',
  extract: 'stage.extract',
  copy: 'stage.copy',
  done: 'stage.done',
  mirror: 'stage.mirror',
  error: 'stage.error',
  canceled: 'stage.canceled',
  metadata: 'stage.metadata',
  client: 'stage.client',
  natives: 'stage.natives',
  logging: 'stage.logging',
  dependencies: 'stage.dependencies',
  upload: 'stage.upload',
  delete: 'stage.delete',
  scan: 'stage.scan',
};

export function TaskTray() {
  const { t, pushToast } = useApp();
  const [minimized, setMinimized] = useState(false);
  const [taskMap, setTaskMap] = useState<Record<string, TaskProgress>>({});
  const [selectedTask, setSelectedTask] = useState<string | null>(null);
  const [taskLog, setTaskLog] = useState<TaskLog | null>(null);
  const [logLoading, setLogLoading] = useState(false);
  const [logError, setLogError] = useState('');
  const selectedRef = useRef<string | null>(null);
  const logBodyRef = useRef<HTMLDivElement | null>(null);
  const timers = useRef(new Map<string, number>());

  useEffect(() => {
    selectedRef.current = selectedTask;
  }, [selectedTask]);

  useEffect(() => {
    const disposeProgress = onEvent<TaskProgress>('task:progress', (task) => {
      setTaskMap((current) => {
        if (current[task.taskId]?.done && !task.done) return current;
        return { ...current, [task.taskId]: task };
      });
      const existing = timers.current.get(task.taskId);
      if (existing) window.clearTimeout(existing);
      if (!task.done) {
        timers.current.delete(task.taskId);
        return;
      }
      const timer = window.setTimeout(() => {
        if (selectedRef.current === task.taskId) return;
        setTaskMap((current) => {
          const next = { ...current };
          delete next[task.taskId];
          return next;
        });
        timers.current.delete(task.taskId);
      }, task.failed ? 8000 : 4200);
      timers.current.set(task.taskId, timer);
    });
    const disposeLog = onEvent<TaskLogEvent>('task:log', (event) => {
      if (selectedRef.current !== event.taskId) return;
      setTaskLog((current) => {
        if (!current || current.taskId !== event.taskId) return current;
        if (current.entries.some((entry) => entry.seq === event.entry.seq)) return current;
        return {
          ...current,
          done: current.done || event.entry.done,
          failed: current.failed || event.entry.failed,
          canceled: current.canceled || event.entry.canceled,
          entries: [...current.entries, event.entry],
        };
      });
    });
    const activeTimers = timers.current;
    return () => {
      disposeProgress();
      disposeLog();
      activeTimers.forEach((timer) => window.clearTimeout(timer));
      activeTimers.clear();
    };
  }, []);

  useEffect(() => {
    const body = logBodyRef.current;
    if (body) body.scrollTop = body.scrollHeight;
  }, [taskLog?.entries.length]);

  const tasks = Object.values(taskMap);
  if (tasks.length === 0) return null;

  const stageLabel = (stage: string) => {
    const key = stage.includes(':') ? stage.split(':').pop()! : stage;
    return t(stageKeys[key] ?? 'stage.processing');
  };

  const removeFinishedTask = (taskId: string | null) => {
    if (!taskId) return;
    setTaskMap((current) => {
      if (!current[taskId]?.done) return current;
      const next = { ...current };
      delete next[taskId];
      return next;
    });
  };

  const closeLog = () => {
    removeFinishedTask(selectedRef.current);
    setSelectedTask(null);
    setTaskLog(null);
    setLogError('');
  };

  const toggleTask = async (task: TaskProgress) => {
    if (selectedRef.current === task.taskId) {
      closeLog();
      return;
    }
    removeFinishedTask(selectedRef.current);
    selectedRef.current = task.taskId;
    setSelectedTask(task.taskId);
    setTaskLog(null);
    setLogError('');
    setLogLoading(true);
    try {
      const log = await api.getTaskLog(task.taskId);
      if (selectedRef.current === task.taskId) setTaskLog(log);
    } catch (reason) {
      if (selectedRef.current === task.taskId) setLogError((reason as Error).message);
    } finally {
      if (selectedRef.current === task.taskId) setLogLoading(false);
    }
  };

  const cancelTask = async (task: TaskProgress) => {
    if (task.done) return;
    // Cancellation is safe (completed work is kept), so do not interrupt the
    // user with a confirmation dialog.
    try {
      await api.cancelTask(task.taskId);
    } catch (reason) {
      pushToast('error', t('task.cancelFailed'), (reason as Error).message);
    }
  };

  const hasFailure = tasks.some((task) => task.done && task.failed && !task.canceled);
  const hasActive = tasks.some((task) => !task.done);
  const hasCanceled = tasks.some((task) => task.done && task.canceled);
  const stateIcon = hasFailure
    ? <TriangleAlert size={16} />
    : hasActive
      ? <LoaderCircle size={16} className="spin" />
      : hasCanceled
        ? <Ban size={16} />
        : <CheckCircle2 size={16} />;

  if (minimized) {
    return (
      <button
        type="button"
        className="task-tray-minimized"
        onClick={() => setMinimized(false)}
        title={t('topbar.restore')}
        aria-label={t('topbar.restore')}
        aria-expanded="false"
      >
        <span className={`task-state-icon ${hasFailure ? 'is-failed' : ''}`}>
          {stateIcon}
        </span>
        <span className="task-tray-minimized-copy">{t('task.background')}</span>
        <span className="task-tray-count">{tasks.length}</span>
        <ChevronUp size={15} />
      </button>
    );
  }

  return (
    <aside className="task-tray" aria-live="polite" aria-label={t('task.background')}>
      <div className="task-tray-head">
        <span className="eyebrow"><span className="eyebrow-dash" />{t('task.background')}</span>
        <div className="task-tray-actions">
          <span className="task-tray-count">{tasks.length}</span>
          <button
            type="button"
            className="icon-button icon-button-sm task-tray-toggle"
            onClick={() => setMinimized(true)}
            title={t('topbar.minimize')}
            aria-label={t('topbar.minimize')}
            aria-expanded="true"
          >
            <ChevronDown size={15} />
          </button>
        </div>
      </div>
      <div className="task-list">
        {tasks.map((task) => {
          const canceled = task.done && task.canceled;
          const failed = task.done && task.failed && !canceled;
          const success = task.done && !failed && !canceled;
          const percent = Math.max(0, Math.min(100, task.percent));
          const selected = selectedTask === task.taskId;
          return (
            <div
              key={task.taskId}
              className={`task-item ${success ? 'is-success' : ''} ${failed ? 'is-failed' : ''} ${canceled ? 'is-canceled' : ''} ${selected ? 'is-open' : ''}`}
            >
              <div
                className="task-item-summary"
                role="button"
                tabIndex={0}
                aria-expanded={selected}
                onClick={() => void toggleTask(task)}
                onKeyDown={(event) => {
                  if (event.key === 'Enter' || event.key === ' ') {
                    event.preventDefault();
                    void toggleTask(task);
                  }
                }}
              >
                <div className="task-item-top">
                  <div className="task-state-icon">
                    {canceled ? <Ban size={16} /> : failed ? <TriangleAlert size={16} /> : success ? <CheckCircle2 size={16} /> : <LoaderCircle size={16} className="spin" />}
                  </div>
                  <div className="task-item-copy">
                    <strong>{task.title}</strong>
                    <span title={task.message}>{task.message}</span>
                  </div>
                  {!task.done && (
                    <button
                      type="button"
                      className="task-cancel"
                      onClick={(event) => { event.stopPropagation(); void cancelTask(task); }}
                      title={t('task.cancel')}
                      aria-label={t('task.cancel')}
                    >
                      <Ban size={13} />
                    </button>
                  )}
                  <span className="task-stage">{canceled ? t('task.canceled') : failed ? t('task.failed') : success ? t('task.done') : stageLabel(task.stage)}</span>
                </div>
                <div className="task-progress">
                  <div
                    className={`task-progress-bar ${task.percent < 0 && !task.done ? 'indeterminate' : ''}`}
                    style={{ width: `${task.percent < 0 ? 38 : percent}%` }}
                  />
                </div>
              </div>

              {selected && (
                <div className="task-log">
                  <div className="task-log-head">
                    <span><FileText size={13} /> {t('task.currentLog')}</span>
                    <button
                      type="button"
                      className="task-log-close"
                      onClick={(event) => { event.stopPropagation(); closeLog(); }}
                      aria-label={t('common.close')}
                    >
                      <X size={13} />
                    </button>
                  </div>
                  <div className="task-log-body" ref={logBodyRef}>
                    {logLoading && <div className="task-log-empty">{t('task.loadingLog')}</div>}
                    {logError && <div className="task-log-empty is-error">{logError}</div>}
                    {!logLoading && !logError && taskLog?.entries.length === 0 && (
                      <div className="task-log-empty">{t('task.noLogs')}</div>
                    )}
                    {taskLog?.entries.map((entry) => (
                      <div key={`${task.taskId}-${entry.seq}`} className={`task-log-line ${entry.failed ? 'is-error' : ''} ${entry.canceled ? 'is-canceled' : ''}`}>
                        <span className="task-log-time">{entry.time}</span>
                        <span className="task-log-stage">{stageLabel(entry.stage)}</span>
                        <span className="task-log-message">{entry.message}</span>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
    </aside>
  );
}
