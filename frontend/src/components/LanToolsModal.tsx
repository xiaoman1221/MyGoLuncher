import { useEffect, useMemo, useRef, useState } from 'react';
import {
  Download,
  FileUp,
  FolderOpen,
  LoaderCircle,
  MessageCircle,
  Mic,
  MicOff,
  Paperclip,
  PhoneOff,
  Plus,
  Send,
  Users,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import { useLanVoice } from '../lib/use-lan-voice';
import { Modal } from './Modal';
import type { LanChatGroup, LanFileInfo, LanPeer, LanToolMessage, LanToolState } from '../lib/types';

type TabKey = 'chat' | 'voice' | 'file';
const GROUP_KEY = 'mygoluncher-lan-groups';
const MAX_VOICE_PEERS = 8;

interface LanToolsModalProps {
  peers: LanPeer[];
  onClose: () => void;
}

function formatBytes(value = 0): string {
  if (value >= 1024 * 1024) return (value / (1024 * 1024)).toFixed(1) + ' MB';
  if (value >= 1024) return (value / 1024).toFixed(0) + ' KB';
  return value + ' B';
}

function formatClock(value: string, locale: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString(locale);
}

function appendMessage(messages: LanToolMessage[], message: LanToolMessage): LanToolMessage[] {
  return [...messages.filter((item) => item.id !== message.id), message].slice(-200);
}

function readGroups(): LanChatGroup[] {
  try {
    const raw = localStorage.getItem(GROUP_KEY);
    const parsed = raw ? JSON.parse(raw) : [];
    if (!Array.isArray(parsed)) return [];
    return parsed.filter((item) => item && typeof item.id === 'string' && typeof item.name === 'string' && Array.isArray(item.peerIds))
      .map((item) => ({ id: item.id, name: item.name, peerIds: item.peerIds.map(String) }));
  } catch {
    return [];
  }
}

function RemoteAudio({ stream }: { stream: MediaStream }) {
  const ref = useRef<HTMLAudioElement>(null);
  useEffect(() => {
    if (ref.current) ref.current.srcObject = stream;
    return () => { if (ref.current) ref.current.srcObject = null; };
  }, [stream]);
  return <audio ref={ref} autoPlay playsInline className="lan-voice-audio" />;
}

export function LanToolsModal({ peers, onClose }: LanToolsModalProps) {
  const { pushToast, t, locale } = useApp();
  const [tab, setTab] = useState<TabKey>('chat');
  const [state, setState] = useState<LanToolState | null>(null);
  const [activeKey, setActiveKey] = useState('all');
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<LanFileInfo | null>(null);
  const [downloading, setDownloading] = useState('');
  const [groups, setGroups] = useState<LanChatGroup[]>(readGroups);
  const [groupOpen, setGroupOpen] = useState(false);
  const [groupName, setGroupName] = useState('');
  const [groupPeers, setGroupPeers] = useState<string[]>([]);
  const historyRef = useRef<HTMLDivElement>(null);

  const chatPeers = useMemo(() => peers.filter((peer) => peer.canChat), [peers]);
  const selfId = state?.selfId ?? '';
  const messages = state?.messages ?? [];
  const voice = useLanVoice({ selfId, onError: (message) => pushToast('error', t('lan.toolsVoiceFailed'), message) });

  useEffect(() => {
    localStorage.setItem(GROUP_KEY, JSON.stringify(groups));
  }, [groups]);

  useEffect(() => {
    let alive = true;
    api.getLanTools()
      .then((current) => { if (alive) setState(current); })
      .catch((error: Error) => { if (alive) pushToast('error', t('lan.toolsUnavailable'), error.message); });
    return () => { alive = false; };
  }, [pushToast, t]);

  useEffect(() => {
    const unsubscribe = onEvent<LanToolMessage>('lan:tool', (message) => {
      setState((current) => current ? { ...current, messages: appendMessage(current.messages ?? [], message) } : current);
      if (message.chatId?.startsWith('group:') && message.chatName) {
        const groupId = message.chatId.slice('group:'.length);
        setGroups((current) => current.some((item) => item.id === groupId) ? current : [...current, {
          id: groupId,
          name: message.chatName || t('lan.toolsGroupFallback'),
          peerIds: (message.members ?? [message.fromId]).filter((id) => id && id !== selfId),
        }]);
      }
    });
    return unsubscribe;
  }, [selfId, t]);

  useEffect(() => {
    const unsubscribe = onEvent<LanToolMessage>('lan:tool:update', (message) => {
      setState((current) => current ? { ...current, messages: appendMessage(current.messages ?? [], message) } : current);
    });
    return unsubscribe;
  }, []);

  useEffect(() => {
    if (!state) return;
    setGroups((current) => {
      let next = current;
      for (const message of state.messages ?? []) {
        if (!message.chatId?.startsWith('group:') || !message.chatName) continue;
        const groupId = message.chatId.slice('group:'.length);
        if (next.some((item) => item.id === groupId)) continue;
        next = [...next, {
          id: groupId,
          name: message.chatName,
          peerIds: (message.members ?? [message.fromId]).filter((id) => id && id !== state.selfId),
        }];
      }
      return next;
    });
  }, [state]);

  useEffect(() => {
    if (tab === 'chat') historyRef.current?.scrollTo({ top: historyRef.current.scrollHeight, behavior: 'smooth' });
  }, [messages.length, tab]);

  const activeGroup = activeKey.startsWith('group:') ? groups.find((item) => 'group:' + item.id === activeKey) : undefined;
  const activePeer = activeKey.startsWith('dm:') ? chatPeers.find((peer) => 'dm:' + peer.id === activeKey) : undefined;
  const activeChat = useMemo(() => {
    if (activeGroup) {
      const ids = activeGroup.peerIds.filter((id) => chatPeers.some((peer) => peer.id === id));
      return {
        key: 'group:' + activeGroup.id,
        chatId: 'group:' + activeGroup.id,
        name: activeGroup.name,
        peerIds: ids,
        members: [selfId, ...ids].filter(Boolean),
      };
    }
    if (activePeer) {
      const ids = [selfId, activePeer.id].sort();
      return {
        key: 'dm:' + activePeer.id,
        chatId: 'dm:' + ids.join(':'),
        name: activePeer.name,
        peerIds: [activePeer.id],
        members: ids.filter(Boolean),
      };
    }
    const ids = chatPeers.map((peer) => peer.id);
    return {
      key: 'all',
      chatId: 'all',
      name: t('lan.toolsEveryone'),
      peerIds: ids,
      members: [selfId, ...ids].filter(Boolean),
    };
  }, [activeGroup, activePeer, chatPeers, selfId, t]);

  useEffect(() => {
    const available = activeKey === 'all'
      || (activeKey.startsWith('dm:') && chatPeers.some((peer) => 'dm:' + peer.id === activeKey))
      || groups.some((item) => 'group:' + item.id === activeKey);
    if (!available) setActiveKey('all');
  }, [activeKey, groups, chatPeers]);

  // Without selfId a private chat id would be malformed (dm::peer), so wait
  // until the toolbox state has loaded.
  const canSend = Boolean(selfId) && activeChat.peerIds.length > 0;
  const chatMessages = messages.filter((message) => {
    if (message.kind !== 'chat') return false;
    if (activeChat.chatId === 'all') return !message.chatId || message.chatId === 'all';
    return message.chatId === activeChat.chatId;
  });
  const fileMessages = messages.filter((message) => {
    if (message.kind !== 'file') return false;
    if (activeChat.chatId === 'all') return !message.chatId || message.chatId === 'all';
    return message.chatId === activeChat.chatId;
  });
  const voicePeers = activeChat.peerIds.map((id) => ({
    id,
    name: chatPeers.find((peer) => peer.id === id)?.name ?? t('lan.toolsPeer'),
  }));

  const sendChat = async () => {
    const value = text.trim();
    if (!value || busy || !canSend) return;
    setBusy(true);
    try {
      const result = await api.sendLanChat(activeChat.peerIds, activeChat.chatId, activeChat.name, activeChat.members, value);
      setState((current) => current ? { ...current, messages: appendMessage(current.messages ?? [], result.message) } : current);
      setText('');
      pushToast('success', t('lan.toolsSent'), t('lan.toolsSentCount', { count: result.count }));
      if (result.warning) pushToast('info', t('lan.toolsPartial'), result.warning);
    } catch (error) {
      pushToast('error', t('lan.toolsSendFailed'), (error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const chooseFile = async () => {
    try {
      const selected = await api.pickLanFile();
      if (selected.path) setFile(selected);
    } catch (error) {
      pushToast('error', t('lan.toolsChooseFailed'), (error as Error).message);
    }
  };

  const sendFile = async () => {
    if (!file || busy || !canSend) return;
    setBusy(true);
    try {
      const result = await api.sendLanFile(activeChat.peerIds, activeChat.chatId, activeChat.name, activeChat.members, file.path);
      setState((current) => current ? { ...current, messages: appendMessage(current.messages ?? [], result.message) } : current);
      setFile(null);
      pushToast('success', t('lan.toolsSent'), t('lan.toolsSentCount', { count: result.count }));
      if (result.warning) pushToast('info', t('lan.toolsPartial'), result.warning);
    } catch (error) {
      pushToast('error', t('lan.toolsSendFailed'), (error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const downloadFile = async (message: LanToolMessage) => {
    setDownloading(message.id);
    try {
      const updated = await api.downloadLanFile(message.id);
      setState((current) => current ? { ...current, messages: appendMessage(current.messages ?? [], updated) } : current);
      pushToast('success', t('lan.toolsDownloaded'), message.fileName);
    } catch (error) {
      pushToast('error', t('lan.toolsDownloadFailed'), (error as Error).message);
    } finally {
      setDownloading('');
    }
  };

  const startVoice = async () => {
    if (voicePeers.length === 0) return;
    if (voicePeers.length > MAX_VOICE_PEERS) {
      pushToast('error', t('lan.toolsVoiceMax'), String(MAX_VOICE_PEERS));
      return;
    }
    try {
      await voice.start('voice:' + activeChat.chatId, voicePeers);
      pushToast('success', t('lan.toolsVoiceStarted'), activeChat.name);
    } catch (error) {
      pushToast('error', t('lan.toolsVoiceFailed'), (error as Error).message);
    }
  };

  const createGroup = () => {
    const name = groupName.trim();
    if (!name || groupPeers.length === 0) {
      pushToast('error', t('lan.toolsGroupInvalid'));
      return;
    }
    const id = typeof crypto !== 'undefined' && 'randomUUID' in crypto
      ? crypto.randomUUID()
      : String(Date.now());
    setGroups((current) => [...current, { id, name, peerIds: groupPeers }]);
    setActiveKey('group:' + id);
    setGroupName('');
    setGroupPeers([]);
    setGroupOpen(false);
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={t('lan.toolsTitle')}
      eyebrow="LAN TOOLS"
      description={t('lan.toolsBody')}
      size="xl"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={() => void api.openLanInbox().catch((error: Error) => pushToast('error', t('lan.toolsOpenInboxFailed'), error.message))}>
            <FolderOpen size={15} /> {t('lan.toolsOpenInbox')}
          </button>
          <button type="button" className="btn btn-primary" onClick={onClose}>{t('common.close')}</button>
        </>
      }
    >
      <div className="lan-tools">
        <div className="lan-tools-tabs">
          <button type="button" className={tab === 'chat' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('chat')}><MessageCircle size={15} /> {t('lan.toolsChat')}</button>
          <button type="button" className={tab === 'voice' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('voice')}><Mic size={15} /> {t('lan.toolsVoice')}</button>
          <button type="button" className={tab === 'file' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('file')}><FileUp size={15} /> {t('lan.toolsFiles')}</button>
        </div>

        <div className="lan-tools-target-row">
          <label className="field lan-tools-target">
            <span className="field-label"><Users size={13} /> {t('lan.toolsTarget')}</span>
            <select className="text-input" value={activeKey} onChange={(event) => setActiveKey(event.target.value)} disabled={voice.active}>
              <option value="all">{t('lan.toolsEveryone')}</option>
              {chatPeers.map((peer) => <option key={peer.id} value={'dm:' + peer.id}>{t('lan.toolsPrivate')} · {peer.name}</option>)}
              {groups.map((group) => <option key={group.id} value={'group:' + group.id}>{t('lan.toolsGroup')} · {group.name}</option>)}
            </select>
          </label>
          <button type="button" className="btn btn-ghost btn-sm" onClick={() => setGroupOpen((value) => !value)} disabled={voice.active}>
            <Plus size={14} /> {t('lan.toolsGroupNew')}
          </button>
        </div>

        {groupOpen && (
          <div className="lan-group-create">
            <input className="text-input" value={groupName} onChange={(event) => setGroupName(event.target.value)} placeholder={t('lan.toolsGroupName')} maxLength={32} />
            <div className="lan-group-peers">
              {chatPeers.map((peer) => (
                <label key={peer.id} className="chip chip-button">
                  <input
                    type="checkbox"
                    checked={groupPeers.includes(peer.id)}
                    onChange={(event) => setGroupPeers((current) => event.target.checked ? [...current, peer.id] : current.filter((id) => id !== peer.id))}
                  />
                  {peer.name}
                </label>
              ))}
            </div>
            <div className="lan-group-actions">
              <button type="button" className="btn btn-ghost btn-sm" onClick={() => setGroupOpen(false)}>{t('common.cancel')}</button>
              <button type="button" className="btn btn-primary btn-sm" onClick={createGroup}><Plus size={14} /> {t('lan.toolsGroupCreate')}</button>
            </div>
          </div>
        )}

        {!canSend && (
          <div className="inline-banner banner-warning">
            <Users size={16} />
            <div><strong>{t('lan.toolsNoPeers')}</strong><span>{t('lan.toolsNoPeersBody')}</span></div>
          </div>
        )}

        {tab === 'chat' && (
          <>
            <div className="lan-tool-history" ref={historyRef}>
              {chatMessages.length === 0 ? <div className="lan-tool-empty">{t('lan.toolsEmpty')}</div> : chatMessages.map((message) => (
                <div className={message.incoming ? 'lan-tool-message' : 'lan-tool-message is-self'} key={message.id}>
                  <div className="lan-tool-message-meta"><strong>{message.fromName}</strong><span>{formatClock(message.createdAt, locale)}</span></div>
                  <p>{message.text}</p>
                </div>
              ))}
            </div>
            <div className="lan-tool-composer">
              <input
                className="text-input"
                value={text}
                placeholder={t('lan.toolsPlaceholder')}
                onChange={(event) => setText(event.target.value)}
                onKeyDown={(event) => {
                  if (event.nativeEvent.isComposing || event.keyCode === 229) return;
                  if (event.key === 'Enter') { event.preventDefault(); void sendChat(); }
                }}
                disabled={busy || !canSend}
              />
              <button type="button" className="btn btn-primary" onClick={() => void sendChat()} disabled={busy || !canSend || !text.trim()}>
                {busy ? <LoaderCircle size={15} className="spin" /> : <Send size={15} />}
                {busy ? t('lan.toolsSending') : t('lan.toolsSend')}
              </button>
            </div>
          </>
        )}

        {tab === 'voice' && (
          <div className="lan-voice-panel">
            {voice.active ? (
              <>
                <div className="lan-voice-live">
                  <div>
                    <span className="lan-voice-dot" />
                    <strong>{t('lan.toolsVoiceLive')} · {activeChat.name}</strong>
                    <small>{t('lan.toolsVoiceParticipants', { count: voice.participants.length })}</small>
                  </div>
                  <button type="button" className="btn btn-danger btn-sm" onClick={voice.stop}><PhoneOff size={14} /> {t('lan.toolsVoiceLeave')}</button>
                </div>
                <div className="lan-voice-actions">
                  <button type="button" className={voice.muted ? 'btn btn-danger btn-sm' : 'btn btn-ghost btn-sm'} onClick={voice.toggleMute}>
                    {voice.muted ? <MicOff size={14} /> : <Mic size={14} />}
                    {voice.muted ? t('lan.toolsVoiceUnmute') : t('lan.toolsVoiceMute')}
                  </button>
                  <span>{t('lan.toolsVoiceLiveHint')}</span>
                </div>
                <div className="lan-voice-participants">
                  {voice.participants.map((peer) => <span key={peer.id} className="chip">{peer.name}</span>)}
                </div>
                {Object.entries(voice.remoteStreams).map(([id, stream]) => <RemoteAudio key={id} stream={stream} />)}
              </>
            ) : (
              <div className="lan-voice-start">
                <div className="lan-voice-start-icon"><Mic size={26} /></div>
                <div>
                  <strong>{t('lan.toolsVoiceStart')}</strong>
                  <span>{t('lan.toolsVoiceStartHint', { name: activeChat.name, count: voicePeers.length })}</span>
                  <small>{t('lan.toolsVoiceRealtimeHint')}</small>
                </div>
                <button type="button" className="btn btn-primary" onClick={() => void startVoice()} disabled={!canSend}>
                  <Mic size={15} /> {t('lan.toolsVoiceStart')}
                </button>
              </div>
            )}
          </div>
        )}

        {tab === 'file' && (
          <div className="lan-tool-file-panel">
            <div className="lan-tool-file-picker">
              <div>
                <strong>{file ? file.name : t('lan.toolsNoFile')}</strong>
                <span>{file ? formatBytes(file.size) : t('lan.toolsFileHint')}</span>
              </div>
              <div className="lan-tool-file-actions">
                <button type="button" className="btn btn-ghost" onClick={() => void chooseFile()} disabled={busy}><Paperclip size={15} /> {t('lan.toolsChooseFile')}</button>
                <button type="button" className="btn btn-primary" onClick={() => void sendFile()} disabled={busy || !file || !canSend}>
                  {busy ? <LoaderCircle size={15} className="spin" /> : <Send size={15} />} {t('lan.toolsSend')}
                </button>
              </div>
            </div>
            <div className="lan-tool-history">
              {fileMessages.length === 0 ? <div className="lan-tool-empty">{t('lan.toolsFileEmpty')}</div> : fileMessages.map((message) => (
                <div className={message.incoming ? 'lan-tool-message' : 'lan-tool-message is-self'} key={message.id}>
                  <div className="lan-tool-message-meta"><strong>{message.fromName}</strong><span>{formatClock(message.createdAt, locale)}</span></div>
                  <div className="lan-tool-file">
                    <Paperclip size={16} />
                    <div><strong>{message.fileName || t('lan.toolsFile')}</strong><span>{formatBytes(message.size ?? 0)}</span></div>
                    {message.incoming && !message.downloaded && (
                      <button type="button" className="btn btn-ghost btn-sm" onClick={() => void downloadFile(message)} disabled={downloading === message.id}>
                        {downloading === message.id ? <LoaderCircle size={13} className="spin" /> : <Download size={13} />}
                        {t('lan.toolsDownload')}
                      </button>
                    )}
                    {message.incoming && message.downloaded && <span className="chip">{t('lan.toolsDownloaded')}</span>}
                  </div>
                  {message.incoming && !message.downloaded && typeof message.progress === 'number' && message.progress > 0 && (
                    <div className="lan-file-progress"><span style={{ width: message.progress + '%' }} /></div>
                  )}
                </div>
              ))}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
