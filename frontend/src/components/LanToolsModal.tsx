import { useEffect, useMemo, useRef, useState } from 'react';
import {
  FileUp,
  FolderOpen,
  LoaderCircle,
  MessageCircle,
  Mic,
  Paperclip,
  Play,
  Send,
  Square,
  Users,
  Volume2,
} from 'lucide-react';
import { api } from '../lib/api';
import { useApp } from '../lib/app-context';
import { onEvent } from '../lib/events';
import { Modal } from './Modal';
import type { LanFileInfo, LanPeer, LanToolMessage, LanToolState } from '../lib/types';

type TabKey = 'chat' | 'voice' | 'file';

interface LanToolsModalProps {
  peers: LanPeer[];
  onClose: () => void;
}

function formatBytes(value = 0): string {
  if (value >= 1024 * 1024) return (value / (1024 * 1024)).toFixed(1) + ' MB';
  if (value >= 1024) return (value / 1024).toFixed(0) + ' KB';
  return String(value) + ' B';
}

function formatClock(value: string, locale: string): string {
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? value : date.toLocaleTimeString(locale);
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = '';
  const chunk = 0x8000;
  for (let index = 0; index < bytes.length; index += chunk) {
    binary += String.fromCharCode(...bytes.subarray(index, index + chunk));
  }
  return btoa(binary);
}

function base64ToBytes(value: string): Uint8Array {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index += 1) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

function appendMessage(messages: LanToolMessage[], message: LanToolMessage): LanToolMessage[] {
  const next = messages.filter((item) => item.id !== message.id);
  next.push(message);
  return next.slice(-200);
}

export function LanToolsModal({ peers, onClose }: LanToolsModalProps) {
  const { pushToast, t, locale } = useApp();
  const [tab, setTab] = useState<TabKey>('chat');
  const [state, setState] = useState<LanToolState | null>(null);
  const [target, setTarget] = useState('');
  const [text, setText] = useState('');
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<LanFileInfo | null>(null);
  const [recording, setRecording] = useState(false);
  const [recordSeconds, setRecordSeconds] = useState(0);
  const [playing, setPlaying] = useState<string | null>(null);
  const historyRef = useRef<HTMLDivElement>(null);
  const recorderRef = useRef<MediaRecorder | null>(null);
  const chunksRef = useRef<Blob[]>([]);
  const streamRef = useRef<MediaStream | null>(null);
  const timerRef = useRef<number | null>(null);
  const startedRef = useRef(0);
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const audioUrls = useRef(new Map<string, string>());

  const chatPeers = useMemo(() => peers.filter((peer) => peer.canChat), [peers]);
  const messages = state?.messages ?? [];
  const canSend = chatPeers.length > 0;

  useEffect(() => {
    let alive = true;
    api.getLanTools()
      .then((current) => { if (alive) setState(current); })
      .catch((error: Error) => { if (alive) pushToast('error', t('lan.toolsUnavailable'), error.message); });
    return () => { alive = false; };
  }, [pushToast, t]);

  useEffect(() => {
    const unsubscribe = onEvent<LanToolMessage>('lan:tool', (message) => {
      setState((current) => current
        ? { ...current, messages: appendMessage(current.messages, message) }
        : current);
    });
    return unsubscribe;
  }, []);

  useEffect(() => {
    if (target && !chatPeers.some((peer) => peer.id === target)) setTarget('');
  }, [chatPeers, target]);

  useEffect(() => {
    if (tab === 'chat') {
      historyRef.current?.scrollTo({ top: historyRef.current.scrollHeight, behavior: 'smooth' });
    }
  }, [messages.length, tab]);

  useEffect(() => () => {
    if (timerRef.current !== null) window.clearInterval(timerRef.current);
    recorderRef.current?.state !== 'inactive' && recorderRef.current?.stop();
    streamRef.current?.getTracks().forEach((track) => track.stop());
    audioRef.current?.pause();
    audioUrls.current.forEach((url) => URL.revokeObjectURL(url));
  }, []);

  const addMessage = (message: LanToolMessage) => {
    setState((current) => current
      ? { ...current, messages: appendMessage(current.messages, message) }
      : current);
  };

  const sendChat = async () => {
    const value = text.trim();
    if (!value || busy || !canSend) return;
    setBusy(true);
    try {
      const result = await api.sendLanChat(target, value);
      addMessage(result.message);
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
      const result = await api.sendLanFile(target, file.path);
      addMessage(result.message);
      setFile(null);
      pushToast('success', t('lan.toolsSent'), t('lan.toolsSentCount', { count: result.count }));
      if (result.warning) pushToast('info', t('lan.toolsPartial'), result.warning);
    } catch (error) {
      pushToast('error', t('lan.toolsSendFailed'), (error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const stopRecording = () => {
    if (timerRef.current !== null) {
      window.clearInterval(timerRef.current);
      timerRef.current = null;
    }
    setRecording(false);
    if (recorderRef.current && recorderRef.current.state !== 'inactive') {
      recorderRef.current.stop();
    }
  };

  const uploadVoice = async (mime: string) => {
    const chunks = chunksRef.current;
    if (chunks.length === 0) return;
    const blob = new Blob(chunks, { type: mime || 'audio/webm' });
    if (blob.size === 0) return;
    setBusy(true);
    try {
      const data = bytesToBase64(new Uint8Array(await blob.arrayBuffer()));
      const duration = startedRef.current ? Date.now() - startedRef.current : recordSeconds * 1000;
      const result = await api.sendLanVoice(target, data, blob.type, duration);
      addMessage(result.message);
      pushToast('success', t('lan.toolsSent'), t('lan.toolsSentCount', { count: result.count }));
      if (result.warning) pushToast('info', t('lan.toolsPartial'), result.warning);
    } catch (error) {
      pushToast('error', t('lan.toolsSendFailed'), (error as Error).message);
    } finally {
      setBusy(false);
    }
  };

  const startRecording = async () => {
    if (recording) {
      stopRecording();
      return;
    }
    if (!canSend) return;
    if (!navigator.mediaDevices?.getUserMedia || typeof MediaRecorder === 'undefined') {
      pushToast('error', t('lan.toolsMicUnsupported'));
      return;
    }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      streamRef.current = stream;
      const preferred = ['audio/webm;codecs=opus', 'audio/webm', 'audio/ogg;codecs=opus']
        .find((mime) => MediaRecorder.isTypeSupported(mime));
      const recorder = new MediaRecorder(stream, preferred ? { mimeType: preferred } : undefined);
      chunksRef.current = [];
      recorder.ondataavailable = (event) => {
        if (event.data.size > 0) chunksRef.current.push(event.data);
      };
      recorder.onstop = () => {
        stream.getTracks().forEach((track) => track.stop());
        streamRef.current = null;
        recorderRef.current = null;
        if (timerRef.current !== null) {
          window.clearInterval(timerRef.current);
          timerRef.current = null;
        }
        setRecording(false);
        void uploadVoice(recorder.mimeType || 'audio/webm');
      };
      recorder.start();
      recorderRef.current = recorder;
      startedRef.current = Date.now();
      setRecordSeconds(0);
      setRecording(true);
      timerRef.current = window.setInterval(() => {
        const elapsed = Math.floor((Date.now() - startedRef.current) / 1000);
        setRecordSeconds(elapsed);
        if (elapsed >= 60) stopRecording();
      }, 500);
    } catch (error) {
      streamRef.current?.getTracks().forEach((track) => track.stop());
      streamRef.current = null;
      pushToast('error', t('lan.toolsMicDenied'), (error as Error).message);
    }
  };

  const playVoice = async (message: LanToolMessage) => {
    if (playing === message.id) {
      audioRef.current?.pause();
      audioRef.current = null;
      setPlaying(null);
      return;
    }
    try {
      let url = audioUrls.current.get(message.id);
      if (!url) {
        const data = await api.getLanVoice(message.id);
        const bytes = base64ToBytes(data);
        const buffer = new ArrayBuffer(bytes.byteLength);
        new Uint8Array(buffer).set(bytes);
        const blob = new Blob([buffer], { type: message.mimeType || 'audio/webm' });
        url = URL.createObjectURL(blob);
        audioUrls.current.set(message.id, url);
      }
      audioRef.current?.pause();
      const audio = new Audio(url);
      audioRef.current = audio;
      audio.onended = () => {
        audioRef.current = null;
        setPlaying(null);
      };
      await audio.play();
      setPlaying(message.id);
    } catch (error) {
      pushToast('error', t('lan.toolsPlayFailed'), (error as Error).message);
    }
  };

  const renderMessage = (message: LanToolMessage) => {
    const self = !message.incoming;
    if (message.kind === 'voice') {
      return (
        <div className={self ? 'lan-tool-message is-self' : 'lan-tool-message'} key={message.id}>
          <div className="lan-tool-message-meta">
            <strong>{message.fromName}</strong>
            <span>{formatClock(message.createdAt, locale)}</span>
          </div>
          <button type="button" className="lan-tool-voice" onClick={() => void playVoice(message)}>
            {playing === message.id ? <Square size={14} /> : <Play size={14} />}
            <Volume2 size={15} />
            <span>{message.durationMs ? (message.durationMs / 1000).toFixed(0) + 's' : t('lan.toolsVoiceMessage')}</span>
            <small>{formatBytes(message.size ?? 0)}</small>
          </button>
        </div>
      );
    }
    if (message.kind === 'file') {
      return (
        <div className={self ? 'lan-tool-message is-self' : 'lan-tool-message'} key={message.id}>
          <div className="lan-tool-message-meta">
            <strong>{message.fromName}</strong>
            <span>{formatClock(message.createdAt, locale)}</span>
          </div>
          <div className="lan-tool-file">
            <Paperclip size={16} />
            <div>
              <strong>{message.fileName || t('lan.toolsFile')}</strong>
              <span>{formatBytes(message.size ?? 0)}</span>
            </div>
            {message.incoming && <span className="chip">{t('lan.toolsReceived')}</span>}
          </div>
        </div>
      );
    }
    return (
      <div className={self ? 'lan-tool-message is-self' : 'lan-tool-message'} key={message.id}>
        <div className="lan-tool-message-meta">
          <strong>{message.fromName}</strong>
          <span>{formatClock(message.createdAt, locale)}</span>
        </div>
        <p>{message.text}</p>
      </div>
    );
  };

  return (
    <Modal
      open
      onClose={onClose}
      title={t('lan.toolsTitle')}
      eyebrow="LAN TOOLS"
      description={t('lan.toolsBody')}
      size="lg"
      footer={
        <>
          <button type="button" className="btn btn-ghost" onClick={() => void api.openLanInbox().catch((error: Error) => pushToast('error', t('lan.toolsOpenInboxFailed'), error.message))}>
            <FolderOpen size={15} /> {t('lan.toolsOpenInbox')}
          </button>
          <button type="button" className="btn btn-primary" onClick={onClose}>
            {t('common.close')}
          </button>
        </>
      }
    >
      <div className="lan-tools">
        <div className="lan-tools-tabs">
          <button type="button" className={tab === 'chat' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('chat')}>
            <MessageCircle size={15} /> {t('lan.toolsChat')}
          </button>
          <button type="button" className={tab === 'voice' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('voice')}>
            <Mic size={15} /> {t('lan.toolsVoice')}
          </button>
          <button type="button" className={tab === 'file' ? 'lan-tools-tab is-active' : 'lan-tools-tab'} onClick={() => setTab('file')}>
            <FileUp size={15} /> {t('lan.toolsFiles')}
          </button>
        </div>

        <label className="field lan-tools-target">
          <span className="field-label"><Users size={13} /> {t('lan.toolsTarget')}</span>
          <select className="text-input" value={target} onChange={(event) => setTarget(event.target.value)} disabled={!canSend || busy || recording}>
            <option value="">{t('lan.toolsEveryone')}</option>
            {chatPeers.map((peer) => (
              <option key={peer.id} value={peer.id}>{peer.name} · {peer.instance || 'Minecraft'}</option>
            ))}
          </select>
        </label>

        {!canSend && (
          <div className="inline-banner banner-warning">
            <Users size={16} />
            <div>
              <strong>{t('lan.toolsNoPeers')}</strong>
              <span>{t('lan.toolsNoPeersBody')}</span>
            </div>
          </div>
        )}

        {tab === 'chat' && (
          <>
            <div className="lan-tool-history" ref={historyRef}>
              {messages.length === 0 ? (
                <div className="lan-tool-empty">{t('lan.toolsEmpty')}</div>
              ) : messages.map(renderMessage)}
            </div>
            <div className="lan-tool-composer">
              <input
                className="text-input"
                value={text}
                placeholder={t('lan.toolsPlaceholder')}
                onChange={(event) => setText(event.target.value)}
                onKeyDown={(event) => {
                  if (event.nativeEvent.isComposing || event.keyCode === 229) return;
                  if (event.key === 'Enter') {
                    event.preventDefault();
                    void sendChat();
                  }
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
          <div className="lan-tool-voice-panel">
            <div className={recording ? 'lan-tool-record is-recording' : 'lan-tool-record'}>
              <div>
                <strong>{recording ? t('lan.toolsRecording', { seconds: recordSeconds }) : t('lan.toolsVoice')}</strong>
                <span>{t('lan.toolsVoiceHint')}</span>
              </div>
              <button type="button" className={recording ? 'btn btn-danger' : 'btn btn-primary'} onClick={() => void startRecording()} disabled={busy || !canSend}>
                {recording ? <Square size={15} /> : <Mic size={15} />}
                {recording ? t('lan.toolsStop') : t('lan.toolsRecord')}
              </button>
            </div>
            <div className="lan-tool-history">
              {messages.filter((message) => message.kind === 'voice').length === 0 ? (
                <div className="lan-tool-empty">{t('lan.toolsVoiceEmpty')}</div>
              ) : messages.filter((message) => message.kind === 'voice').map(renderMessage)}
            </div>
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
                <button type="button" className="btn btn-ghost" onClick={() => void chooseFile()} disabled={busy || recording}>
                  <Paperclip size={15} /> {t('lan.toolsChooseFile')}
                </button>
                <button type="button" className="btn btn-primary" onClick={() => void sendFile()} disabled={busy || !file || !canSend}>
                  {busy ? <LoaderCircle size={15} className="spin" /> : <Send size={15} />}
                  {t('lan.toolsSend')}
                </button>
              </div>
            </div>
            <div className="lan-tool-history">
              {messages.filter((message) => message.kind === 'file').length === 0 ? (
                <div className="lan-tool-empty">{t('lan.toolsFileEmpty')}</div>
              ) : messages.filter((message) => message.kind === 'file').map(renderMessage)}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
