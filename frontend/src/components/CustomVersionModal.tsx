import { useEffect, useState } from 'react';
import { FolderOpen, LoaderCircle, Plus } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { Modal } from './Modal';
import { loaderLabel } from '../lib/format';
import type { ExternalImportResponse } from '../lib/types';

const loaders = ['vanilla', 'fabric', 'quilt', 'forge', 'neoforge'] as const;

interface CustomVersionModalProps {
  open: boolean;
  onClose: () => void;
  onImported: (response: ExternalImportResponse) => Promise<void> | void;
}

export function CustomVersionModal({ open, onClose, onImported }: CustomVersionModalProps) {
  const { pushToast, t } = useApp();
  const [name, setName] = useState('');
  const [mcVersion, setMcVersion] = useState('');
  const [loader, setLoader] = useState<string>('vanilla');
  const [loaderVersion, setLoaderVersion] = useState('');
  const [sourceDir, setSourceDir] = useState('');
  const [importing, setImporting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setName('');
    setMcVersion('');
    setLoader('vanilla');
    setLoaderVersion('');
    setSourceDir('');
    setImporting(false);
  }, [open]);

  const pickDir = async () => {
    try {
      const path = await api.pickCustomSourceDirectory();
      if (path) setSourceDir(path);
    } catch (error) {
      pushToast('error', t('local.customPickFailed'), (error as Error).message);
    }
  };

  const submit = async () => {
    if (!mcVersion.trim()) {
      pushToast('error', t('local.customFailed'), t('local.customVersionRequired'));
      return;
    }
    setImporting(true);
    try {
      const response = await api.importCustomInstance({
        name: name.trim(),
        mcVersion: mcVersion.trim(),
        loader,
        loaderVersion: loader === 'vanilla' ? '' : loaderVersion.trim(),
        sourceDir: sourceDir.trim(),
      });
      await onImported(response);
    } catch (error) {
      pushToast('error', t('local.customFailed'), (error as Error).message);
    } finally {
      setImporting(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      closeable={!importing}
      title={t('local.customTitle')}
      eyebrow="CUSTOM VERSION"
      description={t('local.customBody')}
      size="md"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={onClose} disabled={importing}>
            {t('common.cancel')}
          </button>
          <button
            type="button"
            className="btn btn-primary"
            onClick={() => void submit()}
            disabled={importing || !mcVersion.trim()}
          >
            {importing ? <LoaderCircle size={15} className="spin" /> : <Plus size={15} />}
            {importing ? t('local.customImporting') : t('local.customImport')}
          </button>
        </>
      }
    >
      <div className="settings-fields">
        <label className="field">
          <span className="field-label">{t('local.customName')}</span>
          <input
            className="text-input"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={t('local.customNamePlaceholder')}
          />
        </label>
        <div className="settings-grid-2">
          <label className="field">
            <span className="field-label">{t('local.customVersion')}</span>
            <input
              className="text-input mono"
              value={mcVersion}
              onChange={(event) => setMcVersion(event.target.value)}
              placeholder="1.21.1"
            />
          </label>
          <label className="field">
            <span className="field-label">{t('local.customLoader')}</span>
            <select className="text-input" value={loader} onChange={(event) => setLoader(event.target.value)}>
              {loaders.map((value) => (
                <option key={value} value={value}>
                  {loaderLabel(value, t('loader.vanilla'))}
                </option>
              ))}
            </select>
          </label>
        </div>
        <label className="field">
          <span className="field-label">{t('local.customLoaderVersion')}</span>
          <input
            className="text-input mono"
            value={loaderVersion}
            onChange={(event) => setLoaderVersion(event.target.value)}
            placeholder={loader === 'vanilla' ? t('local.customLoaderVersionVanilla') : t('local.customLoaderVersionPlaceholder')}
            disabled={loader === 'vanilla'}
          />
          <span className="field-hint">{t('local.customLoaderVersionHint')}</span>
        </label>
        <div className="field">
          <span className="field-label">{t('local.customSource')}</span>
          <div className="custom-source-row">
            <input
              className="text-input mono"
              value={sourceDir}
              onChange={(event) => setSourceDir(event.target.value)}
              placeholder={t('local.customSourcePlaceholder')}
            />
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => void pickDir()} disabled={importing}>
              <FolderOpen size={14} /> {t('local.customPickDir')}
            </button>
          </div>
          <span className="field-hint">{t('local.customSourceHint')}</span>
        </div>
        <p className="field-hint">{t('local.customHint')}</p>
      </div>
    </Modal>
  );
}
