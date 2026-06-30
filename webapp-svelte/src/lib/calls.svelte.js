// WebRTC call manager. Mirrors the signaling protocol of the reference client and the
// backend (chat/server/calls.go): an invite is a {pub} with head.webrtc='started' carrying
// a Drafty 'VC' entity; offer/answer/ICE travel as topic.videoCall(event, seq, payload) and
// arrive via topic.onInfo with what==='call'.

import { getClient, iceServers, Drafty } from './tinode.js';
import { constraints as deviceConstraints } from './devices.svelte.js';

const DEFAULT_ICE = [{ urls: 'stun:stun.l.google.com:19302' }];

// Reactive call state, shared across the incoming prompt and the call panel.
let _s = $state({
  active: false,
  direction: null,       // 'outgoing' | 'incoming'
  status: 'idle',        // dialing | ringing | incoming | connecting | active | ended
  audioOnly: false,
  peerName: '',
  topic: null,
  seq: -1,
  localStream: null,
  remoteStream: null,
  muted: false,
  videoOff: false,
  screenSharing: false,
  error: '',
});

export const callState = {
  get active() { return _s.active; },
  get direction() { return _s.direction; },
  get status() { return _s.status; },
  get audioOnly() { return _s.audioOnly; },
  get peerName() { return _s.peerName; },
  get localStream() { return _s.localStream; },
  get remoteStream() { return _s.remoteStream; },
  get muted() { return _s.muted; },
  get videoOff() { return _s.videoOff; },
  get screenSharing() { return _s.screenSharing; },
  get error() { return _s.error; },
};

// --- Internal (non-reactive) WebRTC plumbing --------------------------------
let pc = null;
let isCaller = false;
let setupComplete = false;
let iceCache = [];
let cameraTrack = null; // saved camera track while screen sharing

function constraints() {
  return deviceConstraints(_s.audioOnly);
}

function topicObj() {
  return _s.topic ? getClient().getTopic(_s.topic) : null;
}

function reset() {
  if (pc) { try { pc.close(); } catch { /* */ } }
  pc = null;
  isCaller = false;
  setupComplete = false;
  iceCache = [];
  if (_s.localStream) _s.localStream.getTracks().forEach((t) => t.stop());
  cameraTrack = null;
  _s = { ...(_s), active: false, direction: null, status: 'idle', topic: null, seq: -1, localStream: null, remoteStream: null, muted: false, videoOff: false, screenSharing: false };
}

function createPeerConnection() {
  const conf = { iceServers: iceServers() || DEFAULT_ICE };
  const conn = new RTCPeerConnection(conf);
  conn.onicecandidate = (e) => {
    if (e.candidate) topicObj()?.videoCall('ice-candidate', _s.seq, e.candidate.toJSON());
  };
  conn.ontrack = (e) => { _s.remoteStream = e.streams[0]; _s.status = 'active'; };
  conn.onnegotiationneeded = async () => {
    if (!isCaller || conn.signalingState !== 'stable') return;
    try {
      const offer = await conn.createOffer();
      await conn.setLocalDescription(offer);
      topicObj()?.videoCall('offer', _s.seq, conn.localDescription.toJSON());
    } catch (err) { _s.error = String(err); }
  };
  conn.oniceconnectionstatechange = () => {
    if (['failed', 'disconnected', 'closed'].includes(conn.iceConnectionState)) {
      if (conn.iceConnectionState === 'failed') hangup();
    }
  };
  return conn;
}

async function getLocalMedia() {
  if (_s.localStream) return _s.localStream;
  const stream = await navigator.mediaDevices.getUserMedia(constraints());
  _s.localStream = stream;
  return stream;
}

function addLocalTracks() {
  const stream = _s.localStream;
  if (!stream || !pc) return;
  stream.getTracks().forEach((track) => pc.addTrack(track, stream));
}

function drainIce() {
  iceCache.forEach((c) => pc.addIceCandidate(c).catch(() => {}));
  iceCache = [];
}

// --- Public actions ----------------------------------------------------------

// startCall places an outgoing call on the given p2p topic.
export async function startCall(topicName, peerName, audioOnly = false) {
  if (_s.active) return;
  _s = { ..._s, active: true, direction: 'outgoing', status: 'dialing', audioOnly, peerName, topic: topicName, seq: -1, error: '' };
  isCaller = true;
  try {
    const topic = getClient().getTopic(topicName);
    if (!topic.isSubscribed()) await topic.subscribe();
    getClient().onInfoMessage = routeInfo;
    await getLocalMedia();
    const msg = topic.createMessage(Drafty.videoCall(audioOnly), true);
    msg.head = Object.assign(msg.head || {}, { webrtc: 'started', aonly: audioOnly });
    const ctrl = await topic.publishMessage(msg);
    _s.seq = ctrl?.params?.seq ?? -1;
  } catch (err) {
    _s.error = err?.message || String(err);
    hangup();
  }
}

// handleIncoming is invoked by the global onDataMessage hook when an invite arrives.
export async function handleIncoming(topicName, seq, audioOnly, peerName) {
  if (_s.active) {
    // Already busy: politely decline.
    getClient().getTopic(topicName)?.videoCall('hang-up', seq);
    return;
  }
  _s = { ..._s, active: true, direction: 'incoming', status: 'incoming', audioOnly: !!audioOnly, peerName: peerName || 'Unknown', topic: topicName, seq, error: '' };
  isCaller = false;
  const topic = getClient().getTopic(topicName);
  getClient().onInfoMessage = routeInfo;
  topic.videoCall('ringing', seq);
}

// acceptCall answers an incoming call.
export async function acceptCall() {
  if (!_s.active || _s.direction !== 'incoming') return;
  _s.status = 'connecting';
  try {
    const topic = topicObj();
    if (!topic.isSubscribed()) await topic.subscribe();
    await getLocalMedia();
    topic.videoCall('accept', _s.seq);
  } catch (err) {
    _s.error = err?.message || String(err);
    hangup();
  }
}

// declineCall rejects an incoming call before answering.
export function declineCall() {
  if (!_s.active) return;
  topicObj()?.videoCall('hang-up', _s.seq);
  finish();
}

// hangup ends the current call (any state) and notifies the peer.
export function hangup() {
  if (!_s.active) return;
  topicObj()?.videoCall('hang-up', _s.seq);
  finish();
}

function finish() {
  try { getClient().onInfoMessage = undefined; } catch { /* */ }
  reset();
}

// toggleMute enables/disables the local audio track.
export function toggleMute() {
  const track = _s.localStream?.getAudioTracks()[0];
  if (!track) return;
  track.enabled = !track.enabled;
  _s.muted = !track.enabled;
}

// toggleVideo enables/disables the local video track.
export function toggleVideo() {
  const track = _s.localStream?.getVideoTracks()[0];
  if (!track) return;
  track.enabled = !track.enabled;
  _s.videoOff = !track.enabled;
}

// toggleScreenShare swaps the outgoing video track between the camera and the screen.
export async function toggleScreenShare() {
  if (!pc) return;
  const sender = pc.getSenders().find((s) => s.track && s.track.kind === 'video');
  if (!sender) return;
  if (_s.screenSharing) { await stopScreenShare(); return; }
  try {
    const display = await navigator.mediaDevices.getDisplayMedia({ video: true, audio: false });
    const screenTrack = display.getVideoTracks()[0];
    cameraTrack = sender.track;
    await sender.replaceTrack(screenTrack);
    _s.screenSharing = true;
    screenTrack.onended = () => stopScreenShare();
  } catch (e) {
    _s.error = String(e);
  }
}

async function stopScreenShare() {
  const sender = pc?.getSenders().find((s) => s.track && s.track.kind === 'video');
  if (sender && cameraTrack) await sender.replaceTrack(cameraTrack);
  cameraTrack = null;
  _s.screenSharing = false;
}

// --- Signaling router --------------------------------------------------------

async function routeInfo(info) {
  if (!info || info.what !== 'call') return;
  if (info.seq !== _s.seq && _s.seq !== -1) return;
  try {
    switch (info.event) {
      case 'ringing':
        if (isCaller) _s.status = 'ringing';
        break;
      case 'accept':
        if (isCaller) await beginAsCaller();
        break;
      case 'offer':
        if (!isCaller) await handleOffer(info.payload);
        break;
      case 'answer':
        if (isCaller && pc) { await pc.setRemoteDescription(new RTCSessionDescription(info.payload)); setupComplete = true; drainIce(); }
        break;
      case 'ice-candidate': {
        const cand = new RTCIceCandidate(info.payload);
        if (pc && setupComplete) pc.addIceCandidate(cand).catch(() => {});
        else iceCache.push(cand);
        break;
      }
      case 'hang-up':
        finish();
        break;
    }
  } catch (err) {
    _s.error = String(err);
  }
}

async function beginAsCaller() {
  if (pc) return;
  _s.status = 'connecting';
  pc = createPeerConnection();
  await getLocalMedia();
  addLocalTracks(); // triggers onnegotiationneeded -> offer
}

async function handleOffer(payload) {
  if (!pc) pc = createPeerConnection();
  await pc.setRemoteDescription(new RTCSessionDescription(payload));
  await getLocalMedia();
  addLocalTracks();
  const answer = await pc.createAnswer();
  await pc.setLocalDescription(answer);
  topicObj()?.videoCall('answer', _s.seq, pc.localDescription.toJSON());
  setupComplete = true;
  drainIce();
}
