import { useState } from 'react';
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  LoaderCircle,
  TriangleAlert,
} from 'lucide-react';
import type { TaskProgress } from '../lib/types';
import type { MessageKey } from '../lib/i18n';
import { useApp } from '../lib/app-context';

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
  metadata: 'stage.metadata',
  client: 'stage.client',
  natives: 'stage.natives',
  logging: 'stage.logging',
  dependencies: 'stage.dependencies',
  upload: 'stage.upload',
  delete: 'stage.delete',
  scan: 'stage.scan',
};

export function TaskTray({ tasks }: { tasks: TaskProgress[] }) {
  const { t } = useApp();
  const [minimized, setMinimized] = useState(false);
  if (tasks.length === 0) return null;

  const stageLabel = (stage: string) => {
    const key = stage.includes(':') ? stage.split(':').pop()! : stage;
    return t(stageKeys[key] ?? 'stage.processing');
  };

  const hasFailure = tasks.some((task) => task.done && task.failed);
  const hasActive = tasks.some((task) => !task.done);
  const stateIcon = hasFailure
    ? <TriangleAlert size={16} />
    : hasActive
      ? <LoaderCircle size={16} className="spin" />
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
          const success = task.done && !task.failed;
          const failed = task.done && task.failed;
          const percent = Math.max(0, Math.min(100, task.percent));
          return (
            <div key={task.taskId} className={`task-item ${success ? 'is-success' : ''} ${failed ? 'is-failed' : ''}`}>
              <div className="task-item-top">
                <div className="task-state-icon">
                  {failed ? <TriangleAlert size={16} /> : success ? <CheckCircle2 size={16} /> : <LoaderCircle size={16} className="spin" />}
                </div>
                <div className="task-item-copy">
                  <strong>{task.title}</strong>
                  <span title={task.message}>{task.message}</span>
                </div>
                <span className="task-stage">{failed ? t('task.failed') : success ? t('task.done') : stageLabel(task.stage)}</span>
              </div>
              <div className="task-progress">
                <div
                  className={`task-progress-bar ${task.percent < 0 && !task.done ? 'indeterminate' : ''}`}
                  style={{ width: `${task.percent < 0 ? 38 : percent}%` }}
                />
              </div>
            </div>
          );
        })}
      </div>
    </aside>
  );
}
