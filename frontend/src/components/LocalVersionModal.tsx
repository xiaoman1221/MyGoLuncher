import { FolderInput, LoaderCircle, TriangleAlert } from 'lucide-react';
import type { ExternalCandidate, ExternalScanResult } from '../lib/types';
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
  return (
    <Modal
      open={open}
      onClose={onClose}
      title="添加本地版本"
      eyebrow="LOCAL VERSIONS"
      description={`发现 ${entries.length} 个可导入的版本。导入后会复制存档、模组与配置，并准备游戏运行环境。`}
      size="lg"
      footer={
        <>
          <span className="modal-foot-note">
            <TriangleAlert size={14} /> 账户不会迁移，导入后请重新登录。
          </span>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
            <button type="button" className="btn btn-primary" onClick={onImport} disabled={!selectedKey || importing}>
              {importing ? <LoaderCircle size={15} className="spin" /> : <FolderInput size={15} />}
              {importing ? '正在导入…' : '导入所选版本'}
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
                          {candidate.mcVersion || '未知版本'}
                        </span>
                        <span className={cn('loader-pill', `tone-${loaderTone(candidate.loader)}`)}>
                          {loaderLabel(candidate.loader)}
                        </span>
                        {candidate.loaderVersion && <small>{candidate.loaderVersion}</small>}
                      </span>
                      <span className="local-candidate-meta">
                        <span>{candidate.modCount} 个模组</span>
                        <span>{candidate.hasSaves ? '含存档' : '无存档'}</span>
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
