// LiveKit (SFU) group calls — the scalable path for large rooms ("better than Discord").
//
// Each client publishes one camera/mic stream to the LiveKit server, which fans it out to all
// other participants. A short-lived access token is minted by the backend (/v0/livekit/token);
// the API secret never reaches the browser. Falls back to the mesh implementation (groupcall.js)
// when LiveKit is not configured (token endpoint returns 501).

import { fetchLiveKitToken } from './tinode.js';

// livekit-client is heavy (~120 KB gzip); load it lazily only when a group call starts.
let LK = null; // the dynamically-imported livekit-client module
let room = null;
const audioEls = new Map(); // trackSid -> HTMLAudioElement

let _s = $state({
  active: false,
  roomName: null,
  audioOnly: false,
  muted: false,
  videoOff: false,
  screenSharing: false,
  tiles: [], // { id, identity, isLocal, track }
  error: '',
});

export const liveKit = {
  get active() { return _s.active; },
  get roomName() { return _s.roomName; },
  get audioOnly() { return _s.audioOnly; },
  get muted() { return _s.muted; },
  get videoOff() { return _s.videoOff; },
  get screenSharing() { return _s.screenSharing; },
  get tiles() { return _s.tiles; },
  get error() { return _s.error; },
};

function rebuildTiles() {
  if (!room) { _s.tiles = []; return; }
  const tiles = [];
  const lp = room.localParticipant;
  const localVid = [...lp.videoTrackPublications.values()].find((p) => p.track);
  tiles.push({ id: 'local', identity: 'You', isLocal: true, track: localVid?.track || null });
  for (const p of room.remoteParticipants.values()) {
    const vid = [...p.videoTrackPublications.values()].find((pp) => pp.track && pp.isSubscribed);
    tiles.push({ id: p.sid, identity: p.identity, isLocal: false, track: vid?.track || null });
  }
  _s.tiles = tiles;
  _s.muted = !lp.isMicrophoneEnabled;
  _s.videoOff = !lp.isCameraEnabled;
  _s.screenSharing = lp.isScreenShareEnabled;
}

function wire(r) {
  const { RoomEvent, Track } = LK;
  const refresh = () => rebuildTiles();
  r.on(RoomEvent.ParticipantConnected, refresh)
    .on(RoomEvent.ParticipantDisconnected, refresh)
    .on(RoomEvent.LocalTrackPublished, refresh)
    .on(RoomEvent.LocalTrackUnpublished, refresh)
    .on(RoomEvent.TrackMuted, refresh)
    .on(RoomEvent.TrackUnmuted, refresh)
    .on(RoomEvent.TrackUnsubscribed, (track) => {
      if (track.kind === Track.Kind.Audio) {
        const el = audioEls.get(track.sid);
        if (el) { track.detach(el); el.remove(); audioEls.delete(track.sid); }
      }
      refresh();
    })
    .on(RoomEvent.TrackSubscribed, (track) => {
      if (track.kind === Track.Kind.Audio) {
        const el = track.attach(); // plays automatically
        el.style.display = 'none';
        document.body.appendChild(el);
        audioEls.set(track.sid, el);
      }
      refresh();
    })
    .on(RoomEvent.Disconnected, () => _cleanup());
}

// start connects to a LiveKit room. Throws err.code === 501 when LiveKit is not configured.
export async function start(roomName, audioOnly = false) {
  if (_s.active) return;
  const { url, token } = await fetchLiveKitToken(roomName);
  if (!LK) LK = await import('livekit-client');
  room = new LK.Room({ adaptiveStream: true, dynacast: true });
  wire(room);
  await room.connect(url, token);
  await room.localParticipant.setMicrophoneEnabled(true);
  if (!audioOnly) await room.localParticipant.setCameraEnabled(true);
  _s = { ..._s, active: true, roomName, audioOnly, error: '' };
  rebuildTiles();
}

export async function leave() {
  if (room) await room.disconnect();
  _cleanup();
}

function _cleanup() {
  for (const el of audioEls.values()) el.remove();
  audioEls.clear();
  room = null;
  _s = { ..._s, active: false, roomName: null, tiles: [], muted: false, videoOff: false, screenSharing: false };
}

export async function toggleMute() {
  if (!room) return;
  await room.localParticipant.setMicrophoneEnabled(!room.localParticipant.isMicrophoneEnabled);
  rebuildTiles();
}

export async function toggleVideo() {
  if (!room) return;
  await room.localParticipant.setCameraEnabled(!room.localParticipant.isCameraEnabled);
  rebuildTiles();
}

export async function toggleScreenShare() {
  if (!room) return;
  await room.localParticipant.setScreenShareEnabled(!room.localParticipant.isScreenShareEnabled);
  rebuildTiles();
}
