import { useApp } from '../lib/app-context';
import type { UpdateProgress } from '../lib/types';

export function formatUpdateBytes(value: number): string {
  if (!Number.isFinite(value) || value <= 0) return '0 B';
  const units = ['B', 'KB', 'MB', 'GB'];
  let size = value;
  let index = 0;
  while (size >= 1024 && index < units.length - 1) {
    size /= 1024;
    index += 1;
  }
  return `${size.toFixed(size >= 100 || index === 0 ? 0 : 1)} ${units[index]}`;
}

export function UpdateProgressBar({ progress }: { progress: UpdateProgress }) {
  const { t } = useApp();
  const stageLabel = (stage: string) => {
    switch (stage) {
      case 'download': return t('settings.updateStageDownload');
      case 'verify': return t('settings.updateStageVerify');
      case 'extract': return t('settings.updateStageExtract');
      case 'prepare': return t('settings.updateStagePrepare');
      case 'restart': return t('settings.updateStageRestart');
      case 'done': return t('settings.updateStageDone');
      default: return t('settings.updateApplying');
    }
  };
  return (
    <div className="update-progress">
      <div className="update-progress-head">
        <span>{stageLabel(progress.stage)}</span>
        {progress.percent >= 0 && <strong>{Math.round(progress.percent)}%</strong>}
      </div>
      <div className="update-progress-track">
        <span style={{ width: `${Math.max(0, Math.min(100, progress.percent))}%` }} />
      </div>
      {progress.total > 0 && (
        <span className="field-hint">
          {formatUpdateBytes(progress.current)} / {formatUpdateBytes(progress.total)}
        </span>
      )}
    </div>
  );
}
