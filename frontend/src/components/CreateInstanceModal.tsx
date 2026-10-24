import { useEffect, useMemo, useState, type CSSProperties } from 'react';
import { Box, Coffee, Layers3, Rocket, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { cn } from '../lib/format';
import type { VersionView } from '../lib/types';
import { Modal } from './Modal';
import { VersionPicker } from './VersionPicker';

const loaders = [
  { id: 'vanilla', label: '原版', description: '纯净游戏', tone: 'vanilla' },
  { id: 'fabric', label: 'Fabric', description: '轻量模组', tone: 'fabric' },
  { id: 'quilt', label: 'Quilt', description: 'Fabric 分支', tone: 'quilt' },
  { id: 'forge', label: 'Forge', description: '经典模组', tone: 'forge' },
  { id: 'neoforge', label: 'NeoForge', description: '新生代', tone: 'neoforge' },
];

const memorySteps = [2048, 4096, 6144, 8192, 12288, 16384];

export function CreateInstanceModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const { settings, bootstrap, pushToast, refreshInstances, selectInstance, navigate } = useApp();
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

  const submit = () => {
    if (!mcVersion) {
      pushToast('error', '请选择游戏版本');
      return;
    }
    setSubmitting(true);
    onClose();
    pushToast('info', '正在创建版本', '游戏文件会在后台下载。');
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
      pushToast('success', '版本创建完成', `${instance.name} 已准备好。`);
    }).catch((error: Error) => {
      pushToast('error', '创建版本失败', error.message);
    });
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="新建版本"
      eyebrow="NEW VERSION"
      description="选择 Minecraft 版本和加载器。"
      size="lg"
      footer={
        <>
          <span className="modal-foot-note">
            <Coffee size={14} /> 需要 Java {requiredJava}
          </span>
          <div className="modal-actions">
            <button type="button" className="btn btn-ghost" onClick={onClose}>取消</button>
            <button type="button" className="btn btn-primary" disabled={submitting || loadingVersions || !mcVersion} onClick={submit}>
              <Rocket size={16} />
              {submitting ? '正在创建…' : '创建并安装'}
            </button>
          </div>
        </>
      }
    >
      <div className="form-grid">
        <label className="field field-span-2">
          <span className="field-label">版本名称 <small>可选</small></span>
          <input
            className="text-input"
            value={name}
            onChange={(event) => setName(event.target.value)}
            placeholder={`${mcVersion || 'Minecraft'} 冒险`}
            maxLength={60}
          />
        </label>

        <div className="field field-span-2">
          <span className="field-label">Minecraft 版本</span>
          <VersionPicker
            options={versions}
            value={mcVersion}
            onChange={setMcVersion}
            loading={loadingVersions}
          />
          {versionError && (
            <span className="field-error"><TriangleAlert size={13} /> {versionError}</span>
          )}
        </div>

        <div className="field field-span-2">
          <span className="field-label">模组加载器</span>
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
                  <strong>{item.label}</strong>
                  <small>{item.description}</small>
                </span>
              </button>
            ))}
          </div>
        </div>

        {isModded && (
          <label className="field field-span-2">
            <span className="field-label">加载器版本 <small>留空自动选择最新稳定版</small></span>
            <input
              className="text-input mono"
              value={loaderVersion}
              onChange={(event) => setLoaderVersion(event.target.value)}
              placeholder={loader === 'fabric' ? '例如 0.16.10' : loader === 'quilt' ? '例如 0.26.4' : '例如 47.2.0'}
            />
          </label>
        )}

        <div className="field field-span-2">
          <div className="field-label-row">
            <span className="field-label">分配内存</span>
            <span className="memory-value">
              {autoMemory
                ? `自动 · 约 ${((bootstrap?.autoMemoryMB ?? 4096) / 1024).toFixed(1)} GB`
                : `${(memory / 1024).toFixed(memory % 1024 ? 1 : 0)} GB`}
            </span>
          </div>
          <label className="toggle-row">
            <span>
              <strong>自动分配内存</strong>
              <small>根据系统内存自动计算推荐值</small>
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
