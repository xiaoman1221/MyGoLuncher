import { CheckCircle2, LoaderCircle, TriangleAlert } from 'lucide-react';
import type { TaskProgress } from '../lib/types';
import { stageLabel } from '../lib/format';

export function TaskTray({ tasks }: { tasks: TaskProgress[] }) {
  if (tasks.length === 0) return null;
  return (
    <aside className="task-tray" aria-live="polite" aria-label="后台任务">
      <div className="task-tray-head">
        <span className="eyebrow"><span className="eyebrow-dash" />后台任务</span>
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
                <span className="task-stage">{failed ? '失败' : success ? '完成' : stageLabel(task.stage)}</span>
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
