import { useEffect, useMemo, useRef, useState } from 'react';
import { Check, ChevronsUpDown, LoaderCircle, Search } from 'lucide-react';
import type { VersionView } from '../lib/types';
import { useApp } from '../lib/app-context';
import { cn } from '../lib/format';

interface VersionPickerProps {
  options: VersionView[];
  value: string;
  onChange: (value: string) => void;
  loading?: boolean;
  placeholder?: string;
}

export function VersionPicker({ options, value, onChange, loading, placeholder }: VersionPickerProps) {
  const { t } = useApp();
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState('');
  const rootRef = useRef<HTMLDivElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);

  const typeLabel = (type: string) => {
    switch (type) {
      case 'release': return t('version.type.release');
      case 'snapshot': return t('version.type.snapshot');
      case 'old_beta': return t('version.type.old_beta');
      case 'old_alpha': return t('version.type.old_alpha');
      default: return type;
    }
  };

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
          <span className="select-placeholder"><LoaderCircle size={15} className="spin" /> {t('picker.loading')}</span>
        ) : selected ? (
          <span className="version-trigger-value">
            <span className="mc-version">{selected.id}</span>
            <span className={`version-type type-${selected.type}`}>{typeLabel(selected.type)}</span>
            {selected.installed && <span className="installed-tag">{t('picker.installed')}</span>}
          </span>
        ) : (
          <span className="select-placeholder">{placeholder ?? t('picker.select')}</span>
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
              placeholder={t('picker.searchPlaceholder')}
              aria-label={t('picker.searchAria')}
            />
          </div>
          <div className="version-options">
            {filtered.length === 0 && <div className="version-empty">{t('picker.noMatch')}</div>}
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
                  {version.installed && <span className="installed-tag">{t('picker.installed')}</span>}
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
