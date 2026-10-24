import { Check, FolderInput, LoaderCircle, TriangleAlert } from 'lucide-react';
import type { ExternalCandidate, ExternalScanResult } from '../lib/types';
import { useApp } from '../lib/app-context';
import { cn, loaderLabel, loaderTone } from '../lib/format';
import { Modal } from './Modal';

export interface LocalVersionEntry {
  key: string;
  root: string;
  source: string;
  candidate: ExternalCandidate;
}

export function buildLocalEntries(scans: ExternalScanResult[]): LocalVersionEntry[] {
  const entries: LocalVersionEntry[] = [];
  for (const scan of scans) {
    for (const candidate of scan.candidates) {
      entries.push({
        key: `${scan.root}::${candidate.id}`,
        root: scan.root,
        source: scan.source || candidate.source,
        candidate,
      });
    }
  }
  return entries;
}

interface LocalVersionModalProps {
  open: boolean;
  scans: ExternalScanResult[];
  entries: LocalVersionEntry[];
  selectedKeys: Set<string>;
  importing: boolean;
  onToggle: (key: string) => void;
  onSelectAll: () => void;
  onClearSelection: () => void;
  onClose: () => void;
  onImport: () => void;
}

export function LocalVersionModal({
  open,
  scans,
  entries,
  selectedKeys,
  importing,
  onToggle,
  onSelectAll,
  onClearSelection,
  onClose,
  onImport,
}: LocalVersionModalProps) {
  const { t } = useApp();
  return (
    <Modal
      open={open}
      onClose={onClose}
      closeable={!importing}
      title={t('local.title')}
      eyebrow="LOCAL VERSIONS"
      description={t('local.description', { count: entries.length })}
      size="lg"
      footer={
        <>
          <span className="modal-foot-note">
            <TriangleAlert size={14} /> {t('local.footerNote')}
          </span>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose} disabled={importing}>{t('common.cancel')}</button>
            <button
              type="button"
              className="btn btn-primary"
              onClick={onImport}
              disabled={selectedKeys.size === 0 || importing}
            >
              {importing ? <LoaderCircle size={15} className="spin" /> : <FolderInput size={15} />}
              {importing
                ? t('local.importing')
                : t('local.importCount', { count: selectedKeys.size })}
            </button>
          </div>
        </>
      }
    >
      <div className="local-select-bar">
        <span>{t('local.selectedCount', { count: selectedKeys.size })}</span>
        <div className="local-select-actions">
          <button
            type="button"
            className="text-button"
            onClick={onSelectAll}
            disabled={importing || selectedKeys.size === entries.length}
          >
            {t('local.selectAll')}
          </button>
          <button
            type="button"
            className="text-button"
            onClick={onClearSelection}
            disabled={importing || selectedKeys.size === 0}
          >
            {t('local.clearSelection')}
          </button>
        </div>
      </div>
      <div className="local-scan-list">
        {scans.map((scan) => {
          const group = entries.filter((entry) => entry.root === scan.root);
          if (group.length === 0) return null;
          return (
            <div className="local-scan-group" key={scan.root}>
              <div className="local-scan-head">
                <strong>{scan.source}</strong>
                <code title={scan.root}>{scan.root}</code>
              </div>
              {group.map((entry) => {
                const selected = selectedKeys.has(entry.key);
                const candidate = entry.candidate;
                return (
                  <button
                    type="button"
                    key={entry.key}
                    className={cn('local-candidate', selected && 'is-selected')}
                    onClick={() => onToggle(entry.key)}
                    aria-pressed={selected}
                    disabled={importing}
                  >
                    <span className={cn('local-check', selected && 'is-checked')}>
                      {selected && <Check size={12} strokeWidth={3} />}
                    </span>
                    <span className="local-candidate-copy">
                      <span className="local-candidate-title">
                        <strong>{candidate.name}</strong>
                        <span className="version-type">
                          {candidate.mcVersion || t('common.unknown')}
                        </span>
                        <span className={cn('loader-pill', `tone-${loaderTone(candidate.loader)}`)}>
                          {loaderLabel(candidate.loader, t('loader.vanilla'))}
                        </span>
                        {candidate.loaderVersion && <small>{candidate.loaderVersion}</small>}
                      </span>
                      <span className="local-candidate-meta">
                        <span>{t('local.modsCount', { count: candidate.modCount })}</span>
                        <span>{candidate.hasSaves ? t('local.hasSaves') : t('local.noSaves')}</span>
                        <span>{candidate.source}</span>
                      </span>
                      {candidate.warnings.length > 0 && (
                        <span className="local-warning">
                          <TriangleAlert size={11} /> {candidate.warnings[0]}
                        </span>
                      )}
                    </span>
                  </button>
                );
              })}
            </div>
          );
        })}
      </div>
    </Modal>
  );
}
