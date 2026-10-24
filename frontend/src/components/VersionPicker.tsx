import { useEffect, useMemo, useRef, useState } from 'react';
import { Check, ChevronsUpDown, LoaderCircle, Search } from 'lucide-react';
import type { VersionView } from '../lib/types';
import { cn } from '../lib/format';

interface VersionPickerProps {
  options: VersionView[];
  value: string;
  onChange: (value: string) => void;
  loading?: boolean;
  placeholder?: string;
}

function typeLabel(type: string) {
  switch (type) {
    case 'release': return '正式版';
    case 'snapshot': return '快照';
    case 'old_beta': return '远古 Beta';
    case 'old_alpha': return '远古 Alpha';
    default: return type;
  }
}

export function VersionPicker({ options, value, onChange, loading, placeholder }: VersionPickerProps) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    const onPointerDown = (event: MouseEvent) => {
      if (!rootRef.current?.contains(event.target as Node)) setOpen(false);
    };
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onPointerDown);
    document.addEventListener('keydown', onKeyDown);
    const timer = window.setTimeout(() => searchRef.current?.focus(), 40);
    return () => {
      document.removeEventListener('mousedown', onPointerDown);
      document.removeEventListener('keydown', onKeyDown);
      window.clearTimeout(timer);
    };
  }, [open]);

  useEffect(() => {
    if (!open) setQuery('');
  }, [open]);

  const filtered = useMemo(() => {
    const lower = query.trim().toLowerCase();
    if (!lower) return options.slice(0, 300);
    return options.filter((version) =>
      version.id.toLowerCase().includes(lower) || typeLabel(version.type).includes(lower),
    ).slice(0, 300);
  }, [options, query]);

  const selected = options.find((version) => version.id === value);

  return (
    <div className="version-picker" ref={rootRef}>
      <button
        type="button"
        className={cn('select-trigger', open && 'is-open')}
        onClick={() => setOpen((current) => !current)}
        aria-haspopup="listbox"
        aria-expanded={open}
      >
        {loading ? (
          <span className="select-placeholder"><LoaderCircle size={15} className="spin" /> 正在读取版本…</span>
        ) : selected ? (
          <span className="version-trigger-value">
            <span className="mc-version">{selected.id}</span>
            <span className={`version-type type-${selected.type}`}>{typeLabel(selected.type)}</span>
            {selected.installed && <span className="installed-tag">已安装</span>}
          </span>
        ) : (
          <span className="select-placeholder">{placeholder ?? '选择版本'}</span>
        )}
        <ChevronsUpDown size={16} className="select-chevron" />
      </button>

      {open && (
        <div className="version-popover" role="listbox">
          <div className="version-search">
            <Search size={15} />
            <input
              ref={searchRef}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索版本，例如 1.21.1"
              aria-label="搜索 Minecraft 版本"
            />
          </div>
          <div className="version-options">
            {filtered.length === 0 && <div className="version-empty">没有匹配的版本</div>}
            {filtered.map((version) => (
              <button
                key={version.id}
                type="button"
                role="option"
                aria-selected={version.id === value}
                className={cn('version-option', version.id === value && 'is-selected')}
                onClick={() => {
                  onChange(version.id);
                  setOpen(false);
                }}
              >
                <span className="version-option-main">
                  <span className="mc-version">{version.id}</span>
                  <span className={`version-type type-${version.type}`}>{typeLabel(version.type)}</span>
                </span>
                <span className="version-option-meta">
                  {version.installed && <span className="installed-tag">已安装</span>}
                  {version.id === value && <Check size={15} />}
                </span>
              </button>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
