import type { InstanceView } from '../lib/types';
import { useApp } from '../lib/app-context';
import { useFunLine } from '../lib/fun';
import { loaderLabel } from '../lib/format';

interface LaunchOverlayProps {
  instance: InstanceView;
  phase: 'preparing' | 'waiting';
  onDismiss: () => void;
}

const faces = ['front', 'back', 'right', 'left', 'top', 'bottom'];

export function LaunchOverlay({ instance, phase, onDismiss }: LaunchOverlayProps) {
  const { t, language } = useApp();
  const funLine = useFunLine(language);
  return (
    <div className="launch-overlay" role="status" aria-live="polite">
      <div className="launch-glow" aria-hidden="true" />
      <div className="launch-stage">
        <div className="launch-cube-wrap" aria-hidden="true">
          <div className="launch-cube">
            {faces.map((face) => (
              <span className={`launch-face launch-face-${face}`} key={face} />
            ))}
          </div>
        </div>

        <div className="launch-copy">
          <span className="eyebrow">{t('launch.eyebrow')}</span>
          <h2>{instance.name}</h2>
          <p>{t('launch.version', {
            version: instance.mcVersion,
            loader: loaderLabel(instance.loader, t('loader.vanilla')),
          })}</p>
        </div>

        <div className="launch-progress"><span /></div>
        <p className="launch-status">
          {phase === 'preparing' ? t('launch.preparing') : t('launch.waitingWindow')}
        </p>
        <p className="launch-fun">{funLine}</p>
        <p className="launch-hint">{t('launch.hint')}</p>
        <button type="button" className="btn btn-ghost btn-sm" onClick={onDismiss}>
          {t('launch.dismiss')}
        </button>
      </div>
    </div>
  );
}
