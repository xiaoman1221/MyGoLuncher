import { useEffect, useRef, useState } from 'react';
import {
  FolderOpen,
  MoreHorizontal,
  Pencil,
  Play,
  Square,
  TerminalSquare,
  Trash2,
} from 'lucide-react';
import type { InstanceView } from '../lib/types';
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
          {instance.running && <span className="running-pill"><span className="live-pulse" />运行中</span>}
        </div>
        <span className="instance-meta">
          {instance.mcVersion} · {loaderLabel(instance.loader)}
          {instance.loaderVersion ? ` ${instance.loaderVersion}` : ''} · Java {instance.requiredJava}
        </span>
        <span className={cn('instance-status', !instance.versionInstalled && 'is-incomplete')}>
          {instance.versionInstalled ? `上次启动 ${relativeTime(instance.lastPlayed)}` : '游戏文件不完整'}
        </span>
      </div>

      <div className="instance-actions">
        {instance.running ? (
          <button
            type="button"
            className="btn btn-danger-ghost btn-sm"
            onClick={(event) => { event.stopPropagation(); onStop(); }}
          >
            <Square size={12} fill="currentColor" /> 结束
          </button>
        ) : (
          <button
            type="button"
            className="btn btn-primary btn-sm"
            onClick={(event) => { event.stopPropagation(); onLaunch(); }}
          >
            <Play size={13} fill="currentColor" /> 启动
          </button>
        )}
        {instance.running && (
          <button
            type="button"
            className="btn btn-ghost btn-sm"
            onClick={(event) => { event.stopPropagation(); onConsole(); }}
          >
            <TerminalSquare size={13} /> 控制台
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
            aria-label="更多操作"
            aria-expanded={menuOpen}
          >
            <MoreHorizontal size={17} />
          </button>
          {menuOpen && (
            <div className="context-menu" onClick={(event) => event.stopPropagation()}>
              <button type="button" onClick={() => { setMenuOpen(false); onFolder(); }}>
                <FolderOpen size={15} /> 打开游戏目录
              </button>
              <button type="button" onClick={() => { setMenuOpen(false); onEdit(); }}>
                <Pencil size={15} /> 版本设置
              </button>
              <div className="context-separator" />
              <button type="button" className="danger" onClick={() => { setMenuOpen(false); onDelete(); }}>
                <Trash2 size={15} /> 删除版本
              </button>
            </div>
          )}
        </div>
      </div>
    </article>
  );
}
