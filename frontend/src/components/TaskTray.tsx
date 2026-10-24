import { CheckCircle2, LoaderCircle, TriangleAlert } from 'lucide-react';
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
};

export function TaskTray({ tasks }: { tasks: TaskProgress[] }) {
  const { t } = useApp();
  if (tasks.length === 0) return null;

  const stageLabel = (stage: string) => {
    const key = stage.includes(':') ? stage.split(':').pop()! : stage;
    return t(stageKeys[key] ?? 'stage.processing');
  };

  return (
    <aside className="task-tray" aria-live="polite" aria-label={t('task.background')}>
      <div className="task-tray-head">
        <span className="eyebrow"><span className="eyebrow-dash" />{t('task.background')}</span>
        <span className="task-tray-count">{tasks.length}</span>
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
