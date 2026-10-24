import { useCallback, useEffect, useState } from 'react';
import {
  Boxes,
  Coffee,
  Cpu,
  Layers3,
  Play,
  Plus,
  Quote,
  RefreshCw,
  Settings2,
  Square,
  TerminalSquare,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { InstanceCard } from '../components/InstanceCard';
import { EmptyState } from '../components/EmptyState';
import { greeting, loaderLabel, loaderTone, relativeTime } from '../lib/format';
import type { Hitokoto } from '../lib/types';

export function HomePage() {
  const {
    instances, selectedInstance, selectedId, selectInstance, settings, selectedAccount,
    navigate, openCreate, openEdit, askDelete, launch, stop, openConsole, openInstanceFolder,
  } = useApp();
  const [hitokoto, setHitokoto] = useState<Hitokoto | null>(null);
  const [hitokotoLoading, setHitokotoLoading] = useState(true);
  const [hitokotoError, setHitokotoError] = useState('');

  const loadHitokoto = useCallback(async () => {
    setHitokotoLoading(true);
    setHitokotoError('');
    try {
      setHitokoto(await api.getHitokoto());
    } catch (error) {
      setHitokotoError((error as Error).message);
    } finally {
      setHitokotoLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadHitokoto();
  }, [loadHitokoto]);

  const player = selectedAccount?.name ?? settings?.playerName ?? 'Steve';
  const running = instances.filter((instance) => instance.running);
  const hitokotoSource = hitokoto
    ? [hitokoto.from, hitokoto.fromWho].filter(Boolean).join(' · ') || '一言'
    : '';

  return (
    <div className="page home-page">
      <section className="home-head">
        <div>
          <div className="eyebrow"><span className="eyebrow-dash" />MY GO LUNCHER · {greeting()}，{player}</div>
          <h1 className="home-title">下一段冒险，从这里开始。</h1>
          <p>选择一个版本，开始今天的游戏。</p>
        </div>
        <button type="button" className="btn btn-primary" onClick={openCreate}>
          <Plus size={17} /> 新建版本
        </button>
      </section>

      {selectedInstance ? (
        <section className={`play-panel tone-${loaderTone(selectedInstance.loader)}`}>
          <div className="play-copy">
            <div className="play-title">
              <span className="eyebrow">当前版本</span>
              {selectedInstance.running && (
                <span className="running-pill"><span className="live-pulse" />运行中</span>
              )}
            </div>
            <h2>{selectedInstance.name}</h2>
            <div className="meta-row">
              <span className="meta-chip"><Boxes size={13} /> {selectedInstance.mcVersion}</span>
              <span className="meta-chip"><Layers3 size={13} /> {loaderLabel(selectedInstance.loader)}</span>
              <span className="meta-chip"><Coffee size={13} /> Java {selectedInstance.requiredJava}</span>
              <span className="meta-chip">
                <Cpu size={13} />
                {((selectedInstance.effectiveMemoryMB || 0) / 1024).toFixed(0)} GB
              </span>
            </div>
            <p className="play-hint">
              {selectedInstance.versionInstalled
                ? `上次启动：${relativeTime(selectedInstance.lastPlayed)}`
                : '游戏文件不完整，请重新添加或安装该版本。'}
            </p>
          </div>
          <div className="play-actions">
            {selectedInstance.running ? (
              <>
                <button type="button" className="btn btn-primary btn-lg" onClick={() => openConsole(selectedInstance.id)}>
                  <TerminalSquare size={18} /> 打开控制台
                </button>
                <button type="button" className="btn btn-danger-ghost" onClick={() => void stop(selectedInstance.id)}>
                  <Square size={14} fill="currentColor" /> 结束游戏
                </button>
              </>
            ) : (
              <>
                <button type="button" className="btn btn-primary btn-lg" onClick={() => void launch(selectedInstance.id)}>
                  <Play size={18} fill="currentColor" /> 启动游戏
                </button>
                <button type="button" className="btn btn-ghost" onClick={() => openEdit(selectedInstance.id)}>
                  <Settings2 size={16} /> 版本设置
                </button>
              </>
            )}
          </div>
        </section>
      ) : (
        <section className="play-panel play-panel-empty">
          <div>
            <span className="eyebrow">当前版本</span>
            <h2>还没有版本</h2>
            <p>新建一个版本，或从其他启动器导入已有版本。</p>
          </div>
          <div className="play-actions">
            <button type="button" className="btn btn-primary" onClick={openCreate}>
              <Plus size={16} /> 新建版本
            </button>
            <button type="button" className="btn btn-ghost" onClick={() => navigate('versions')}>
              添加本地版本
            </button>
          </div>
        </section>
      )}

      <section className="hitokoto-card">
        <div className="hitokoto-head">
          <span className="eyebrow"><span className="eyebrow-dash" />你知道吗</span>
          <button
            type="button"
            className="icon-button icon-button-sm"
            onClick={() => void loadHitokoto()}
            disabled={hitokotoLoading}
            title="换一句"
            aria-label="换一句一言"
          >
            <RefreshCw size={14} className={hitokotoLoading ? 'spin' : ''} />
          </button>
        </div>
        {hitokoto ? (
          <>
            <p className="hitokoto-text">“{hitokoto.text}”</p>
            <div className="hitokoto-meta">
              <span>—— {hitokotoSource}</span>
              <span className="hitokoto-tag"><Quote size={11} /> 一言</span>
            </div>
          </>
        ) : hitokotoLoading ? (
          <div className="hitokoto-skeleton">
            <span className="skeleton skeleton-line wide" />
            <span className="skeleton skeleton-line short" />
          </div>
        ) : (
          <p className="hitokoto-text is-error" title={hitokotoError}>
            暂时无法获取一言，点击右上角重试。
          </p>
        )}
      </section>

      <section>
        <div className="section-head">
          <div>
            <h2>我的版本 <span className="section-count">{instances.length}</span></h2>
            {running.length > 0 && <span className="section-subtitle">{running.length} 个正在运行</span>}
          </div>
          <div className="section-actions">
            <button type="button" className="btn btn-ghost btn-sm" onClick={() => navigate('versions')}>
              <Boxes size={15} /> 添加版本
            </button>
            <button type="button" className="btn btn-primary btn-sm" onClick={openCreate}>
              <Plus size={15} /> 新建
            </button>
          </div>
        </div>

        {instances.length === 0 ? (
          <EmptyState
            icon={<Boxes size={26} />}
            title="还没有任何版本"
            body="新建一个 Minecraft 版本，或从官方启动器、HMCL、PCL2、Prism 导入。"
            action={<button type="button" className="btn btn-primary" onClick={() => navigate('versions')}><Plus size={16} /> 添加版本</button>}
          />
        ) : (
          <div className="instance-list">
            {instances.map((instance) => (
              <InstanceCard
                key={instance.id}
                instance={instance}
                active={instance.id === selectedId}
                onSelect={() => selectInstance(instance.id)}
                onLaunch={() => void launch(instance.id)}
                onStop={() => void stop(instance.id)}
                onConsole={() => openConsole(instance.id)}
                onFolder={() => void openInstanceFolder(instance.id)}
                onEdit={() => openEdit(instance.id)}
                onDelete={() => askDelete(instance.id)}
              />
            ))}
          </div>
        )}
      </section>
    </div>
  );
}
