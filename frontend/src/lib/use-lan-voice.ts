import { useCallback, useEffect, useRef, useState } from 'react';
import { api } from './api';
import type { LanSignal, LanSignalBatch } from './types';

export interface LanVoicePeer {
  id: string;
  name: string;
}

interface UseLanVoiceOptions {
  selfId: string;
  onError: (message: string) => void;
}

interface PeerState {
  pc: RTCPeerConnection;
  pendingIce: RTCIceCandidateInit[];
  remoteSet: boolean;
}

export function useLanVoice({ selfId, onError }: UseLanVoiceOptions) {
  const [active, setActive] = useState(false);
  const [session, setSession] = useState('');
  const [participants, setParticipants] = useState<LanVoicePeer[]>([]);
  const [muted, setMuted] = useState(false);
  const [remoteStreams, setRemoteStreams] = useState<Record<string, MediaStream>>({});

  const localStreamRef = useRef<MediaStream | null>(null);
  const peersRef = useRef<Map<string, PeerState>>(new Map());
  const sessionRef = useRef('');
  const selfIdRef = useRef(selfId);
  const activeRef = useRef(false);
  const seqRef = useRef(0);
  const pollRef = useRef<number | null>(null);
  const pollingRef = useRef(false);

  selfIdRef.current = selfId;

  const sendSignal = useCallback(async (peerId: string, kind: 'offer' | 'answer' | 'ice', payload: string) => {
    if (!sessionRef.current) return;
    try {
      await api.sendLanSignal(peerId, sessionRef.current, kind, payload);
    } catch (error) {
      onError((error as Error).message);
    }
  }, [onError]);

  const ensurePeer = useCallback((peerId: string): PeerState => {
    const existing = peersRef.current.get(peerId);
    if (existing) return existing;
    const pc = new RTCPeerConnection({ iceServers: [], iceCandidatePoolSize: 4 });
    const state: PeerState = { pc, pendingIce: [], remoteSet: false };
    peersRef.current.set(peerId, state);
    localStreamRef.current?.getTracks().forEach((track) => pc.addTrack(track, localStreamRef.current!));
    pc.onicecandidate = (event) => {
      if (event.candidate) void sendSignal(peerId, 'ice', JSON.stringify(event.candidate));
    };
    pc.ontrack = (event) => {
      const stream = event.streams[0] ?? new MediaStream([event.track]);
      setRemoteStreams((current) => ({ ...current, [peerId]: stream }));
    };
    pc.onconnectionstatechange = () => {
      if (pc.connectionState === 'failed' || pc.connectionState === 'closed') {
        setRemoteStreams((current) => {
          const next = { ...current };
          delete next[peerId];
          return next;
        });
      }
    };
    return state;
  }, [sendSignal]);

  const handleSignal = useCallback(async (signal: LanSignal) => {
    if (!activeRef.current || signal.session !== sessionRef.current || !signal.fromId) return;
    try {
      if (signal.kind === 'offer') {
        const state = ensurePeer(signal.fromId);
        await state.pc.setRemoteDescription(JSON.parse(signal.payload) as RTCSessionDescriptionInit);
        state.remoteSet = true;
        for (const candidate of state.pendingIce.splice(0)) {
          await state.pc.addIceCandidate(candidate).catch(() => {});
        }
        const answer = await state.pc.createAnswer();
        await state.pc.setLocalDescription(answer);
        await sendSignal(signal.fromId, 'answer', JSON.stringify(answer));
        return;
      }
      if (signal.kind === 'answer') {
        const state = peersRef.current.get(signal.fromId);
        if (!state) return;
        await state.pc.setRemoteDescription(JSON.parse(signal.payload) as RTCSessionDescriptionInit);
        state.remoteSet = true;
        for (const candidate of state.pendingIce.splice(0)) {
          await state.pc.addIceCandidate(candidate).catch(() => {});
        }
        return;
      }
      if (signal.kind === 'ice') {
        const state = ensurePeer(signal.fromId);
        const candidate = JSON.parse(signal.payload) as RTCIceCandidateInit;
        if (state.remoteSet) {
          await state.pc.addIceCandidate(candidate).catch(() => {});
        } else {
          state.pendingIce.push(candidate);
        }
      }
    } catch (error) {
      onError((error as Error).message);
    }
  }, [ensurePeer, onError, sendSignal]);

  const pollSignals = useCallback(async () => {
    if (!activeRef.current || pollingRef.current) return;
    pollingRef.current = true;
    try {
      const batch: LanSignalBatch = await api.pollLanSignals(seqRef.current);
      seqRef.current = batch.lastSeq;
      for (const signal of batch.signals) await handleSignal(signal);
    } catch (error) {
      // A transient poll error should not terminate the call; the next tick retries.
      if (activeRef.current) onError((error as Error).message);
    } finally {
      pollingRef.current = false;
    }
  }, [handleSignal, onError]);

  const stop = useCallback(() => {
    activeRef.current = false;
    if (pollRef.current !== null) {
      window.clearInterval(pollRef.current);
      pollRef.current = null;
    }
    for (const state of peersRef.current.values()) {
      state.pc.close();
    }
    peersRef.current.clear();
    localStreamRef.current?.getTracks().forEach((track) => track.stop());
    localStreamRef.current = null;
    setRemoteStreams({});
    setActive(false);
    setParticipants([]);
    setMuted(false);
    setSession('');
    sessionRef.current = '';
  }, []);

  const start = useCallback(async (nextSession: string, peers: LanVoicePeer[]) => {
    if (activeRef.current) stop();
    if (!navigator.mediaDevices?.getUserMedia || typeof RTCPeerConnection === 'undefined') {
      throw new Error('当前环境不支持实时语音');
    }
    const stream = await navigator.mediaDevices.getUserMedia({
      audio: { echoCancellation: true, noiseSuppression: true, autoGainControl: true },
    });
    try {
      localStreamRef.current = stream;
      sessionRef.current = nextSession;
      activeRef.current = true;
      seqRef.current = 0;
      setActive(true);
      setSession(nextSession);
      setParticipants(peers);
      setMuted(false);
      pollRef.current = window.setInterval(() => { void pollSignals(); }, 300);

      for (const peer of peers) {
        if (peer.id === selfIdRef.current) continue;
        // Only the lexicographically smaller peer creates the initial offer,
        // avoiding WebRTC glare when both sides start the same conversation.
        if (selfIdRef.current < peer.id) {
          const state = ensurePeer(peer.id);
          const offer = await state.pc.createOffer();
          await state.pc.setLocalDescription(offer);
          await sendSignal(peer.id, 'offer', JSON.stringify(offer));
        }
      }
    } catch (error) {
      stop();
      throw error;
    }
  }, [ensurePeer, pollSignals, sendSignal, stop]);

  const toggleMute = useCallback(() => {
    const stream = localStreamRef.current;
    if (!stream) return;
    const next = !muted;
    stream.getAudioTracks().forEach((track) => { track.enabled = !next; });
    setMuted(next);
  }, [muted]);

  useEffect(() => () => { stop(); }, [stop]);

  return {
    active, session, participants, muted, remoteStreams,
    start, stop, toggleMute,
  };
}
