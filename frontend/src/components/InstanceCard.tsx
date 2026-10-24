import { useEffect, useRef, useState } from 'react';
import {
  FolderOpen,
  LoaderCircle,
  MoreHorizontal,
  Pencil,
  Play,
  Square,
  TerminalSquare,
  Trash2,
} from 'lucide-react';
import type { InstanceView } from '../lib/types';
import { useApp } from '../lib/app-context';
import { cn, loaderLabel, loaderTone, relativeTime } from '../lib/format';

interface InstanceCardProps {
  instance: InstanceView;
  active: boolean;
  onSelect: () => void;
  onLaunch: () => void;
  onStop: () => void;
  onConsole: () => void;
  onFolder: () => void;
  onEdit: () => void;
  onDelete: () => void;
}

export function InstanceCard({
  instance,
  active,
  onSelect,
  onLaunch,
  onStop,
  onConsole,
  onFolder,
  onEdit,
  onDelete,
}: InstanceCardProps) {
  const { t, locale, launchingId } = useApp();
  const launching = launchingId === instance.id;
  const [menuOpen, setMenuOpen] = useState(false);
  const menuRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!menuOpen) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!menuRef.current?.contains(event.target as Node)) setMenuOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setMenuOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
    };
  }, [menuOpen]);

  return (
    <article
      className={cn(
        'instance-card',
        active && 'is-active',
        instance.running && 'is-running',
        `tone-${loaderTone(instance.loader)}`,
      )}
      onClick={onSelect}
    >
      <div className="instance-mark">
        <span>{instance.mcVersion}</span>
      </div>

      <div className="instance-copy">
        <div className="instance-name-row">
          <h3>{instance.name}</h3>
          {instance.running && <span className="running-pill"><span className="live-pulse" />{t('home.running')}</span>}
        </div>
        <span className="instance-meta">
          {instance.mcVersion} · {loaderLabel(instance.loader, t('loader.vanilla'))}
          {instance.loaderVersion ? ` ${instance.loaderVersion}` : ''} · Java {instance.requiredJava}
        </span>
        <span className={cn('instance-status', !instance.versionInstalled && 'is-incomplete')}>
          {instance.versionInstalled
            ? t('home.lastLaunch', { time: relativeTime(instance.lastPlayed, locale, t('common.unknown')) })
            : t('home.filesIncomplete')}
        </span>
      </div>

      <div className="instance-actions">
        {instance.running ? (
          <button
            type="button"
            className="btn btn-danger-ghost btn-sm"
            onClick={(event) => { event.stopPropagation(); onStop(); }}
          >
            <Square size={12} fill="currentColor" /> {t('common.stop')}
          </button>
        ) : (
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={(event) => { event.stopPropagation(); onLaunch(); }}
            disabled={launching}
          >
            {launching
              ? <LoaderCircle size={13} className="spin" />
              : <Play size={13} fill="currentColor" />}
            {launching ? t('launch.button') : t('common.launch')}
          </button>
        )}
        {instance.running && (
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={(event) => { event.stopPropagation(); onConsole(); }}
          >
            <TerminalSquare size={13} /> {t('common.console')}
          </button>
        )}
        <div className="instance-menu-wrap" ref={menuRef}>
          <button
            type="button"
            className="icon-button icon-button-sm"
            onClick={(event) => {
              event.stopPropagation();
              setMenuOpen((value) => !value);
            }}
            aria-label={t('common.moreActions')}
            aria-expanded={menuOpen}
          >
            <MoreHorizontal size={17} />
          </button>
          {menuOpen && (
            <div className="context-menu" onClick={(event) => event.stopPropagation()}>
              <button type="button" onClick={() => { setMenuOpen(false); onFolder(); }}>
                <FolderOpen size={15} /> {t('common.openGameFolder')}
              </button>
              <button type="button" onClick={() => { setMenuOpen(false); onEdit(); }}>
                <Pencil size={15} /> {t('common.versionSettings')}
              </button>
              <div className="context-separator" />
              <button type="button" className="danger" onClick={() => { setMenuOpen(false); onDelete(); }}>
                <Trash2 size={15} /> {t('common.deleteVersion')}
              </button>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
