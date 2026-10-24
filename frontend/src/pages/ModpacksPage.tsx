import { useState } from 'react';
import {
  FileArchive,
  FolderOpen,
  LoaderCircle,
  PackageOpen,
  Server,
  Upload,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { PageHeader } from '../components/PageHeader';
import { EmptyState } from '../components/EmptyState';
import { loaderLabel, loaderTone } from '../lib/format';

export function ModpacksPage() {
  const {
    instances, refreshInstances, selectInstance, pushToast, openInstanceFolder, t,
  } = useApp();
  const [importing, setImporting] = useState(false);
  const [exporting, setExporting] = useState<string | null>(null);

  const importPack = async () => {
    setImporting(true);
    try {
      const response = await api.importModpackDialog();
      if (!response.instance.id) return;
      await refreshInstances();
      selectInstance(response.instance.id);
      const warning = response.warnings.length > 0 ? ` ${response.warnings[0]}` : '';
      pushToast('success', t('modpacks.imported'), `${t('modpacks.importedDetail', { name: response.instance.name })}${warning}`);
    } catch (error) {
      pushToast('error', t('modpacks.importFailed'), (error as Error).message);
    } finally {
      setImporting(false);
    }
  };

  const exportPack = async (id: string) => {
    setExporting(`mrpack:${id}`);
    try {
      const path = await api.exportModpackDialog(id);
      if (path) pushToast('success', t('modpacks.exportDone'), path);
    } catch (error) {
      pushToast('error', t('modpacks.exportFailed'), (error as Error).message);
    } finally {
      setExporting(null);
    }
  };

  const exportBackup = async (id: string) => {
    setExporting(`zip:${id}`);
    try {
      const path = await api.exportInstanceBackupDialog(id);
      if (path) pushToast('success', t('modpacks.exportDone'), path);
    } catch (error) {
      pushToast('error', t('modpacks.exportFailed'), (error as Error).message);
    } finally {
      setExporting(null);
    }
  };

  const exportServer = async (id: string) => {
    setExporting(`server:${id}`);
    try {
      const response = await api.exportServerPackDialog(id);
      if (response.path) {
        const warning = response.warnings.length > 0 ? ` · ${response.warnings[0]}` : '';
        pushToast('success', t('modpacks.exportServerDone'), `${response.path}${warning}`);
      }
    } catch (error) {
      pushToast('error', t('modpacks.exportServerFailed'), (error as Error).message);
    } finally {
      setExporting(null);
    }
  };

  return (
    <div className="page modpacks-page">
      <PageHeader
        kicker="MODPACKS"
        title={<>{t('modpacks.title')} <span className="title-accent">/</span> {t('modpacks.tail')}</>}
        description={t('modpacks.description')}
        actions={
          <button type="button" className="btn btn-primary" onClick={() => void importPack()} disabled={importing}>
            {importing ? <LoaderCircle size={16} className="spin" /> : <Upload size={16} />}
            {importing ? t('modpacks.importing') : t('modpacks.importButton')}
          </button>
        }
      />

      <section className="panel">
        <div className="panel-head">
          <h2>{t('modpacks.importTitle')}</h2>
          <p>{t('modpacks.importBody')}</p>
        </div>
        <div className="panel-actions">
          <button type="button" className="btn btn-primary" onClick={() => void importPack()} disabled={importing}>
            {importing ? <LoaderCircle size={16} className="spin" /> : <Upload size={16} />}
            {importing ? t('modpacks.importing') : t('modpacks.importButton')}
          </button>
        </div>
        <p className="panel-note">{t('modpacks.note')}</p>
      </section>

      <section className="panel">
        <div className="panel-head">
          <h2>{t('modpacks.exportTitle')}</h2>
          <p>{t('modpacks.exportBody')}</p>
        </div>

        {instances.length === 0 ? (
          <EmptyState
            compact
            icon={<PackageOpen size={24} />}
            title={t('modpacks.emptyTitle')}
            body={t('modpacks.emptyBody')}
            action={<button type="button" className="btn btn-primary btn-sm" onClick={() => void importPack()}><Upload size={14} /> {t('modpacks.importButton')}</button>}
          />
        ) : (
          <div className="modpack-export-list">
            {instances.map((instance) => (
              <div className="modpack-export-row" key={instance.id}>
                <div className={`modpack-export-mark tone-${loaderTone(instance.loader)}`}>
                  {instance.mcVersion}
                </div>
                <div className="modpack-export-copy">
                  <strong>{instance.name}</strong>
                  <span>
                    {instance.mcVersion} · {loaderLabel(instance.loader, t('loader.vanilla'))}
                    {instance.loaderVersion ? ` ${instance.loaderVersion}` : ''}
                  </span>
                </div>
                <div className="modpack-export-actions">
                  <button type="button" className="btn btn-ghost btn-sm" onClick={() => void openInstanceFolder(instance.id)}>
                    <FolderOpen size={14} /> {t('common.folder')}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    disabled={Boolean(exporting)}
                    onClick={() => void exportBackup(instance.id)}
                  >
                    {exporting === `zip:${instance.id}` ? <LoaderCircle size={14} className="spin" /> : <FileArchive size={14} />}
                    {t('modpacks.exportZip')}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    disabled={Boolean(exporting)}
                    onClick={() => void exportPack(instance.id)}
                  >
                    {exporting === `mrpack:${instance.id}` ? <LoaderCircle size={14} className="spin" /> : <PackageOpen size={14} />}
                    {t('modpacks.exportMRPack')}
                  </button>
                  <button
                    type="button"
                    className="btn btn-ghost btn-sm"
                    disabled={Boolean(exporting)}
                    onClick={() => void exportServer(instance.id)}
                  >
                    {exporting === `server:${instance.id}` ? <LoaderCircle size={14} className="spin" /> : <Server size={14} />}
                    {t('modpacks.exportServer')}
                  </button>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
