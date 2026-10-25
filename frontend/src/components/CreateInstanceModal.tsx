import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import { Box, Coffee, Layers3, Rocket, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import type { MessageKey } from '../lib/i18n';
import { cn } from '../lib/format';
import type { VersionView } from '../lib/types';
import { Modal } from './Modal';
import { VersionPicker } from './VersionPicker';

const loaders: Array<{ id: string; label: string; descriptionKey: MessageKey; tone: string }> = [
  { id: 'vanilla', label: 'Vanilla', descriptionKey: 'loader.vanillaDesc', tone: 'vanilla' },
  { id: 'fabric', label: 'Fabric', descriptionKey: 'loader.fabricDesc', tone: 'fabric' },
  { id: 'quilt', label: 'Quilt', descriptionKey: 'loader.quiltDesc', tone: 'quilt' },
  { id: 'forge', label: 'Forge', descriptionKey: 'loader.forgeDesc', tone: 'forge' },
  { id: 'neoforge', label: 'NeoForge', descriptionKey: 'loader.neoforgeDesc', tone: 'neoforge' },
];

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];

export function CreateInstanceModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const {
    settings, bootstrap, pushToast, refreshInstances, selectInstance, navigate, t,
    hasFeature, openLicense,
  } = useApp();
  const [versions, setVersions] = useState<VersionView[]>([]);
  const [loadingVersions, setLoadingVersions] = useState(false);
  const [versionError, setVersionError] = useState('');
  const [mcVersion, setMcVersion] = useState('');
  const [name, setName] = useState('');
  const [loader, setLoader] = useState('vanilla');
  const [loaderVersion, setLoaderVersion] = useState('');
  const [memory, setMemory] = useState(4096);
  const [autoMemory, setAutoMemory] = useState(true);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!open) return;
    setSubmitting(false);
    setName('');
    setLoader('vanilla');
    setLoaderVersion('');
    const defaultMemory = settings?.defaultMemoryMB ?? 0;
    setAutoMemory(defaultMemory === 0);
    setMemory(defaultMemory > 0 ? defaultMemory : (bootstrap?.autoMemoryMB ?? 4096));
    setVersionError('');
    setLoadingVersions(true);
    let alive = true;
    api.listVersions(false, false)
      .then((result) => {
        if (!alive) return;
        setVersions(result.versions);
        setMcVersion((current) => current || result.latestRelease);
      })
      .catch((error: Error) => {
        if (alive) setVersionError(error.message);
      })
      .finally(() => {
        if (alive) setLoadingVersions(false);
      });
    return () => { alive = false; };
  }, [open, settings?.defaultMemoryMB, bootstrap?.autoMemoryMB]);

  const selectedVersion = useMemo(
    () => versions.find((version) => version.id === mcVersion),
    [versions, mcVersion],
  );

  const requiredJava = selectedVersion?.javaMajor ?? 21;
  const isModded = loader !== 'vanilla';
  const needsVersionLicense = Boolean(selectedVersion && !selectedVersion.installed && !hasFeature('versions'));

  const submit = () => {
    if (!mcVersion) {
      pushToast('error', t('create.selectVersion'));
      return;
    }
    if (needsVersionLicense) {
      openLicense();
      return;
    }
    setSubmitting(true);
    onClose();
    pushToast('info', t('create.started'), t('create.startedDetail'));
    void api.createInstance({
      name: name.trim(),
      mcVersion,
      loader,
      loaderVersion: loaderVersion.trim(),
      memoryMB: autoMemory ? 0 : memory,
    }).then(async (instance) => {
      await refreshInstances();
      selectInstance(instance.id);
      navigate('home');
      pushToast('success', t('create.done'), t('create.doneDetail', { name: instance.name }));
    }).catch((error: Error) => {
      pushToast('error', t('create.failed'), error.message);
    });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={t('create.title')}
      eyebrow="NEW VERSION"
      description={t('create.description')}
      size="lg"
      footer={
        <>
          <span className="modal-foot-note">
            <Coffee size={14} /> {t('create.requiredJava', { java: requiredJava })}
          </span>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>{t('common.cancel')}</button>
            <button type="button" className="btn btn-primary" disabled={submitting || loadingVersions || !mcVersion} onClick={submit}>
              <Rocket size={16} />
              {submitting ? t('create.creating') : needsVersionLicense ? t('versions.downloadLockedShort') : t('create.createInstall')}
            </button>
          </div>
        </>
      }
    >
      <div className="form-grid">
        <label className="field field-span-2">
          <span className="field-label">{t('create.name')} <small>{t('create.optional')}</small></span>
          <input
            className="text-input"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={t('create.namePlaceholder', { version: mcVersion || 'Minecraft' })}
            maxLength={60}
          />
        </label>

        <div className="field field-span-2">
          <span className="field-label">{t('create.mcVersion')}</span>
          <VersionPicker
            options={versions}
            value={mcVersion}
            onChange={setMcVersion}
            loading={loadingVersions}
          />
          {versionError && (
            <span className="field-error"><TriangleAlert size={13} /> {versionError}</span>
          )}
          {needsVersionLicense && (
            <div className="inline-banner banner-warning">
              <TriangleAlert size={15} />
              <div><strong>{t('versions.downloadLocked')}</strong><span>{t('versions.downloadLockedHint')}</span></div>
              <button type="button" className="btn btn-ghost btn-sm" onClick={openLicense}>
                {t('paid.activate')}
              </button>
            </div>
          )}
        </div>

        <div className="field field-span-2">
          <span className="field-label">{t('create.loader')}</span>
          <div className="loader-grid">
            {loaders.map((item) => (
              <button
                key={item.id}
                type="button"
                className={cn('loader-option', `tone-${item.tone}`, loader === item.id && 'is-active')}
                onClick={() => setLoader(item.id)}
              >
                <span className="loader-symbol">{item.id === 'vanilla' ? <Box size={18} /> : <Layers3 size={18} />}</span>
                <span>
                  <strong>{item.id === 'vanilla' ? t('loader.vanilla') : item.label}</strong>
                  <small>{t(item.descriptionKey)}</small>
                </span>
              </button>
            ))}
          </div>
        </div>

        {isModded && (
          <label className="field field-span-2">
            <span className="field-label">{t('create.loaderVersion')} <small>{t('create.loaderVersionHint')}</small></span>
            <input
              className="text-input mono"
              value={loaderVersion}
              onChange={(event) => setLoaderVersion(event.target.value)}
              placeholder={loader === 'fabric'
                ? t('create.loaderPlaceholder.fabric')
                : loader === 'quilt'
                  ? t('create.loaderPlaceholder.quilt')
                  : t('create.loaderPlaceholder.other')}
            />
          </label>
        )}

        <div className="field field-span-2">
          <div className="field-label-row">
            <span className="field-label">{t('create.memory')}</span>
            <span className="memory-value">
              {autoMemory
                ? t('settings.autoMemoryValue', { gb: ((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1) })
                : `${(memory / 1024).toFixed(memory % 1024 ? 1 : 0)} GB`}
            </span>
          </div>
          <label className="toggle-row">
            <span>
              <strong>{t('create.autoMemory')}</strong>
              <small>{t('create.autoMemoryHint')}</small>
            </span>
            <input
              type="checkbox"
              className="switch"
              checked={autoMemory}
              onChange={(event) => {
                setAutoMemory(event.target.checked);
                if (!event.target.checked) setMemory(bootstrap?.autoMemoryMB ?? 4096);
              }}
            />
          </label>
          {!autoMemory && (
            <>
              <input
                className="range"
                type="range"
                min={1024}
                max={16384}
                step={512}
                value={memory}
                onChange={(event) => setMemory(Number(event.target.value))}
                style={{ '--range-progress': `${((memory - 1024) / (16384 - 1024)) * 100}%` } as CSSProperties}
              />
              <div className="memory-presets">
                {memorySteps.map((value) => (
                  <button
                    key={value}
                    type="button"
                    className={cn('chip', 'chip-button', memory === value && 'is-active')}
                    onClick={() => setMemory(value)}
                  >
                    {value / 1024} GB
                  </button>
                ))}
              </div>
            </>
          )}
        </div>
      </div>
    </Modal>
  );
}
