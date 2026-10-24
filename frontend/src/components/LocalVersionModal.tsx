import { FolderInput, LoaderCircle, TriangleAlert } from 'lucide-react';
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
  selectedKey: string;
  importing: boolean;
  onSelect: (key: string) => void;
  onClose: () => void;
  onImport: () => void;
}

export function LocalVersionModal({
  open,
  scans,
  entries,
  selectedKey,
  importing,
  onSelect,
  onClose,
  onImport,
}: LocalVersionModalProps) {
  const { t } = useApp();
  return (
    <Modal
      open={open}
      onClose={onClose}
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
            <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
            <button type="button" className="btn btn-primary" onClick={onImport} disabled={!selectedKey || importing}>
              {importing ? <LoaderCircle size={15} className="spin" /> : <FolderInput size={15} />}
              {importing ? t('local.importing') : t('local.import')}
            </button>
          </div>
        </>
      }
    >
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
                const selected = entry.key === selectedKey;
                const candidate = entry.candidate;
                return (
                  <button
                    type="button"
                    key={entry.key}
                    className={cn('local-candidate', selected && 'is-selected')}
                    onClick={() => onSelect(entry.key)}
                  >
                    <span className="local-radio">{selected && <span />}</span>
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
