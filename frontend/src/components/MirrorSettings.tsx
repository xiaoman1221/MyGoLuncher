import { useEffect, useState } from 'react';
import { Gauge, Globe2, LoaderCircle, Plus, Trash2, TriangleAlert } from 'lucide-react';
import { api } from '../lib/api';
import type { MirrorConfig, MirrorSpeedResult, ResourceMirror, Settings } from '../lib/types';
import type { MessageKey } from '../lib/i18n';
import { cn } from '../lib/format';

type Translate = (key: MessageKey, vars?: Record<string, string | number>) => string;
type Patch = <K extends keyof Settings>(key: K, value: Settings[K]) => void;

function speedLabel(result: MirrorSpeedResult): string {
  if (!result.ok) return result.error || '-';
  const value = result.bytesPerSec;
  if (value >= 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB/s · ${result.latencyMs} ms`;
  return `${(value / 1024).toFixed(0)} KB/s · ${result.latencyMs} ms`;
}

export function DownloadMirrorSettings({ form, patch, t }: { form: Settings; patch: Patch; t: Translate }) {
  const [testing, setTesting] = useState(false);
  const [results, setResults] = useState<MirrorSpeedResult[]>([]);
  const [error, setError] = useState('');

  const update = (index: number, key: keyof MirrorConfig, value: string) => {
    patch('customMirrors', form.customMirrors.map((item, itemIndex) => (
      itemIndex === index ? { ...item, [key]: value } : item
    )));
  };

  const add = () => {
    patch('customMirrors', [...form.customMirrors, {
      id: `custom-${Date.now()}`, name: '', url: '', kind: 'bmclapi',
    }]);
  };

  const remove = (index: number) => {
    patch('customMirrors', form.customMirrors.filter((_, itemIndex) => itemIndex !== index));
  };

  const test = async () => {
    setTesting(true);
    setError('');
    try {
      setResults(await api.testDownloadMirrors(form.customMirrors));
    } catch (err) {
      setResults([]);
      setError((err as Error).message || t('settings.mirrorTestFailed'));
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="settings-fields">
      <div className="mirror-builtins">
        <span className="chip is-key"><Globe2 size={12} /> {t('settings.mirrorOfficial')}</span>
        <span className="chip">BMCLAPI</span>
        <span className="field-hint">{t('settings.mirrorBuiltinHint')}</span>
      </div>
      <div className="mirror-list">
        {form.customMirrors.map((mirror, index) => (
          <div className="mirror-row" key={mirror.id || index}>
            <input
              className="text-input"
              value={mirror.name}
              placeholder={t('settings.mirrorName')}
              onChange={(event) => update(index, 'name', event.target.value)}
            />
            <input
              className="text-input mono"
              value={mirror.url}
              placeholder="https://bmclapi2.example.com"
              onChange={(event) => update(index, 'url', event.target.value)}
            />
            <select
              className="text-input"
              value={mirror.kind === 'proxy' ? 'proxy' : 'bmclapi'}
              onChange={(event) => update(index, 'kind', event.target.value)}
            >
              <option value="bmclapi">{t('settings.mirrorKindBMCL')}</option>
              <option value="proxy">{t('settings.mirrorKindProxy')}</option>
            </select>
            <button type="button" className="icon-button danger" onClick={() => remove(index)} aria-label={t('common.delete')}>
              <Trash2 size={15} />
            </button>
          </div>
        ))}
      </div>
      <div className="panel-actions">
        <button type="button" className="btn btn-ghost btn-sm" onClick={add}>
          <Plus size={14} /> {t('settings.mirrorAdd')}
        </button>
        <button type="button" className="btn btn-ghost btn-sm" onClick={() => void test()} disabled={testing}>
          {testing ? <LoaderCircle size={14} className="spin" /> : <Gauge size={14} />}
          {testing ? t('settings.mirrorTesting') : t('settings.mirrorTest')}
        </button>
      </div>
      {error && (
        <div className="inline-banner banner-warning">
          <TriangleAlert size={16} />
          <div><span>{t('settings.mirrorTestFailed')}</span><small>{error}</small></div>
        </div>
      )}
      {results.length > 0 && (
        <div className="mirror-results">
          {results.map((result) => (
            <div className={cn('mirror-result', result.ok ? 'is-ok' : 'is-error')} key={result.id}>
              <strong>{result.name}</strong>
              <span>{speedLabel(result)}</span>
            </div>
          ))}
        </div>
      )}
      <p className="field-hint">{t('settings.mirrorHint')}</p>
    </div>
  );
}

export function ResourceMirrorSettings({ form, patch, t }: { form: Settings; patch: Patch; t: Translate }) {
  const [remote, setRemote] = useState<ResourceMirror[]>([]);

  useEffect(() => {
    let alive = true;
    api.listResourceMirrors()
      .then((rows) => { if (alive) setRemote(rows); })
      .catch(() => { if (alive) setRemote([]); });
    return () => { alive = false; };
  }, []);

  const options = (() => {
    const merged = new Map<string, ResourceMirror>();
    for (const source of remote) merged.set(source.id, source);
    for (const item of form.customResourceMirrors) {
      if (!merged.has(item.id)) {
        merged.set(item.id, {
          id: item.id, name: item.name || item.url, baseUrl: item.url,
          kind: item.kind, official: false, custom: true,
        });
      }
    }
    if (!merged.has('official')) {
      merged.set('official', {
        id: 'official', name: t('settings.mirrorOfficial'), baseUrl: '',
        kind: 'official', official: true, custom: false,
      });
    }
    if (!merged.has('mcim')) {
      merged.set('mcim', {
        id: 'mcim', name: 'MCIM', baseUrl: 'https://mod.mcimirror.top',
        kind: 'mcim', official: false, custom: false,
      });
    }
    return Array.from(merged.values());
  })();

  const update = (index: number, key: keyof MirrorConfig, value: string) => {
    patch('customResourceMirrors', form.customResourceMirrors.map((item, itemIndex) => (
      itemIndex === index ? { ...item, [key]: value } : item
    )));
  };

  const add = () => {
    patch('customResourceMirrors', [...form.customResourceMirrors, {
      id: `resource-${Date.now()}`, name: '', url: '', kind: 'mcim',
    }]);
  };

  const remove = (index: number) => {
    const removed = form.customResourceMirrors[index];
    patch('customResourceMirrors', form.customResourceMirrors.filter((_, itemIndex) => itemIndex !== index));
    if (removed && form.resourceMirror === removed.id) patch('resourceMirror', 'official');
  };

  return (
    <div className="settings-fields">
      <label className="field">
        <span className="field-label">{t('settings.resourceMirrorDefault')}</span>
        <select
          className="text-input"
          value={form.resourceMirror || 'official'}
          onChange={(event) => patch('resourceMirror', event.target.value)}
        >
          {options.map((source) => (
            <option key={source.id} value={source.id}>
              {source.name}{source.custom ? ' · ' + t('settings.mirrorCustom') : ''}
            </option>
          ))}
        </select>
        <span className="field-hint">{t('settings.resourceMirrorHint')}</span>
      </label>
      <div className="mirror-list">
        {form.customResourceMirrors.map((mirror, index) => (
          <div className="mirror-row" key={mirror.id || index}>
            <input
              className="text-input"
              value={mirror.name}
              placeholder={t('settings.mirrorName')}
              onChange={(event) => update(index, 'name', event.target.value)}
            />
            <input
              className="text-input mono"
              value={mirror.url}
              placeholder="https://mod.mcimirror.top"
              onChange={(event) => update(index, 'url', event.target.value)}
            />
            <select
              className="text-input"
              value={mirror.kind === 'proxy' ? 'proxy' : 'mcim'}
              onChange={(event) => update(index, 'kind', event.target.value)}
            >
              <option value="mcim">MCIM</option>
              <option value="proxy">{t('settings.mirrorKindProxy')}</option>
            </select>
            <button type="button" className="icon-button danger" onClick={() => remove(index)} aria-label={t('common.delete')}>
              <Trash2 size={15} />
            </button>
          </div>
        ))}
      </div>
      <div className="panel-actions">
        <button type="button" className="btn btn-ghost btn-sm" onClick={add}>
          <Plus size={14} /> {t('settings.mirrorAdd')}
        </button>
      </div>
      <p className="field-hint">{t('settings.resourceMirrorCustomHint')}</p>
    </div>
  );
}
