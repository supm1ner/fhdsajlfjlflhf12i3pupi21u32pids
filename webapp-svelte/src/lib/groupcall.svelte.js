// Group (multi-party) calls via a full WebRTC mesh — no backend changes.
//
// The built-in call signaling (calls.go) is strictly 1:1, so group calls use a separate
// signaling channel: messages published to the group topic carrying an `mcall` head
// ({join,offer,answer,ice,leave} + from/to + payload). Each participant keeps one
// RTCPeerConnection per remote peer. Glare is avoided by the rule: when a peer announces
// `join`, only the *existing* members send it an offer (the joiner just answers).
//
// Mesh scales to small groups (~4-6). For Discord-scale rooms an SFU (LiveKit/mediasoup)
// is the real path — see docs. Signaling rides on stored messages (filtered from the feed);
// an SFU or a dedicated signaling verb would remove that.

import { getClient, iceServers, myUID } from './tinode.js';
import { constraints as deviceConstraints } from './devices.svelte.js';

const DEFAULT_ICE = [{ urls: 'stun:stun.l.google.com:19302' }];

let _s = $state({
  active: false,
  topic: null,
  audioOnly: false,
  muted: false,
  videoOff: false,
  screenSharing: false,
  // participants: array of { uid, stream } (remote peers). Local preview is separate.
  peers: [],
  localStream: null,
  error: '',
});

export const groupCall = {
  get active() { return _s.active; },
  get topic() { return _s.topic; },
  get audioOnly() { return _s.audioOnly; },
  get muted() { return _s.muted; },
  get videoOff() { return _s.videoOff; },
  get screenSharing() { return _s.screenSharing; },
  get peers() { return _s.peers; },
  get localStream() { return _s.localStream; },
  get error() { return _s.error; },
};

// uid -> { pc, stream }
const conns = new Map();
let cameraTrack = null;

function topicObj() { return _s.topic ? getClient().getTopic(_s.topic) : null; }

// signal publishes a head-only mesh message to the group topic (noecho: not echoed to us).
function signal(head) {
  const t = topicObj();
  if (!t) return;
  const msg = t.createMessage(' ', true);
  msg.head = Object.assign(msg.head || {}, head);
  t.publishMessage(msg).catch(() => {});
}

function syncPeers() {
  _s.peers = [...conns.entries()].map(([uid, c]) => ({ uid, stream: c.stream }));
}

function makePc(peerUid) {
  const pc = new RTCPeerConnection({ iceServers: iceServers() || DEFAULT_ICE });
  const entry = { pc, stream: null };
  conns.set(peerUid, entry);

  pc.onicecandidate = (e) => {
    if (e.candidate) signal({ mcall: 'ice', to: peerUid, from: myUID(), cand: e.candidate.toJSON() });
  };
  pc.ontrack = (e) => {
    entry.stream = e.streams[0];
    syncPeers();
  };
  pc.onconnectionstatechange = () => {
    if (['failed', 'closed'].includes(pc.connectionState)) removePeer(peerUid);
  };
  // Push our local tracks to this peer.
  if (_s.localStream) _s.localStream.getTracks().forEach((t) => pc.addTrack(t, _s.localStream));
  return entry;
}

function removePeer(uid) {
  const c = conns.get(uid);
  if (c) { try { c.pc.close(); } catch { /* */ } conns.delete(uid); syncPeers(); }
}

// --- Public actions ----------------------------------------------------------

export async function startGroupCall(topicName, audioOnly = false) {
  if (_s.active) return;
  _s = { ..._s, active: true, topic: topicName, audioOnly, muted: false, videoOff: false, screenSharing: false, error: '', peers: [] };
  try {
    const t = getClient().getTopic(topicName);
    if (!t.isSubscribed()) await t.subscribe();
    _s.localStream = await navigator.mediaDevices.getUserMedia(deviceConstraints(audioOnly));
    // Announce our presence; existing members will offer to us.
    signal({ mcall: 'join', from: myUID() });
  } catch (e) {
    _s.error = e?.message || String(e);
    leaveGroupCall();
  }
}

export function leaveGroupCall() {
  if (_s.topic) signal({ mcall: 'leave', from: myUID() });
  for (const uid of [...conns.keys()]) removePeer(uid);
  if (_s.localStream) _s.localStream.getTracks().forEach((t) => t.stop());
  cameraTrack = null;
  _s = { ..._s, active: false, topic: null, peers: [], localStream: null, muted: false, videoOff: false, screenSharing: false };
}

export function toggleMute() {
  const track = _s.localStream?.getAudioTracks()[0];
  if (!track) return;
  track.enabled = !track.enabled;
  _s.muted = !track.enabled;
}

export function toggleVideo() {
  const track = _s.localStream?.getVideoTracks()[0];
  if (!track) return;
  track.enabled = !track.enabled;
  _s.videoOff = !track.enabled;
}

export async function toggleScreenShare() {
  if (_s.screenSharing) { await stopScreenShare(); return; }
  try {
    const display = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
    const screenTrack = display.getVideoTracks()[0];
    cameraTrack = _s.localStream?.getVideoTracks()[0] || null;
    // Replace the outgoing video track on every peer connection.
    for (const { pc } of conns.values()) {
      const sender = pc.getSenders().find((s) => s.track && s.track.kind === 'video');
      if (sender) await sender.replaceTrack(screenTrack);
    }
    _s.screenSharing = true;
    screenTrack.onended = () => stopScreenShare();
  } catch (e) {
    _s.error = String(e);
  }
}

async function stopScreenShare() {
  if (cameraTrack) {
    for (const { pc } of conns.values()) {
      const sender = pc.getSenders().find((s) => s.track && s.track.kind === 'video');
      if (sender) await sender.replaceTrack(cameraTrack);
    }
  }
  cameraTrack = null;
  _s.screenSharing = false;
}

// --- Signaling router (called from the global onDataMessage hook) ------------

export async function handleSignal(head, fromUid) {
  if (!_s.active || !head || !head.mcall) return;
  if (fromUid === myUID()) return;
  // Messages addressed to a specific peer that isn't us are ignored.
  if (head.to && head.to !== myUID()) return;

  try {
    switch (head.mcall) {
      case 'join': {
        // A new participant joined; as an existing member, offer to them.
        const entry = makePc(fromUid);
        const offer = await entry.pc.createOffer();
        await entry.pc.setLocalDescription(offer);
        signal({ mcall: 'offer', to: fromUid, from: myUID(), sdp: entry.pc.localDescription.toJSON() });
        break;
      }
      case 'offer': {
        const entry = conns.get(fromUid) || makePc(fromUid);
        await entry.pc.setRemoteDescription(new RTCSessionDescription(head.sdp));
        const answer = await entry.pc.createAnswer();
        await entry.pc.setLocalDescription(answer);
        signal({ mcall: 'answer', to: fromUid, from: myUID(), sdp: entry.pc.localDescription.toJSON() });
        break;
      }
      case 'answer': {
        const entry = conns.get(fromUid);
        if (entry) await entry.pc.setRemoteDescription(new RTCSessionDescription(head.sdp));
        break;
      }
      case 'ice': {
        const entry = conns.get(fromUid);
        if (entry && head.cand) await entry.pc.addIceCandidate(new RTCIceCandidate(head.cand)).catch(() => {});
        break;
      }
      case 'leave':
        removePeer(fromUid);
        break;
    }
  } catch (e) {
    _s.error = String(e);
  }
}
