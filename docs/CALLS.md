# Calls — 1:1, group, screen share, device settings

The messenger supports WebRTC calls beyond the backend's built-in 1:1 protocol.

## 1:1 calls
Use the backend signaling in `chat/server/calls.go`: an invite is a `{pub}` with
`head.webrtc='started'` + a Drafty `VC` entity; `offer/answer/ICE` travel via
`topic.videoCall(event, seq, payload)` and arrive on the global `onInfoMessage` hook.
Web: `src/lib/calls.svelte.js` + `CallPanel`/`IncomingCall`. Flutter:
`call_controller.dart` + `CallScreen`/`IncomingCallView`.

## Screen sharing
`getDisplayMedia()` captures the screen; the outgoing video track is swapped via
`RTCRtpSender.replaceTrack()`, so no renegotiation churn. Toggling again restores the camera.
Available in 1:1 (web + Flutter) and in group calls (web).

## Device settings (PC)
`src/lib/devices.svelte.js` enumerates microphones, speakers and cameras
(`enumerateDevices`), remembers the choice (localStorage), builds `getUserMedia`
constraints, and routes audio output with `HTMLMediaElement.setSinkId()` (Chromium/desktop).
Toggles for noise suppression / echo cancellation / auto-gain. UI: `CallSettings.svelte`
(reachable from the in-call ⚙️ button).

## Group calls (mesh)
The backend call protocol is strictly 1:1, so group calls use a **full WebRTC mesh** with a
separate signaling channel — messages published to the group topic carrying an `mcall` head
(`join/offer/answer/ice/leave` + `from`/`to` + SDP/ICE). Each participant holds one
`RTCPeerConnection` per peer; glare is avoided by having only existing members offer to a
newcomer. Implementation: `src/lib/groupcall.svelte.js` + `GroupCallPanel`/`VideoTile`.
Signaling messages are filtered out of the chat feed.

Mesh is simple and serverless but each client uploads its stream to every peer, so it scales to
~4-6 participants. For large rooms use **LiveKit** (below). The group-call button prefers LiveKit and
falls back to mesh automatically when LiveKit isn't configured.

## Group calls (LiveKit SFU) — the scalable path

For Discord-scale rooms the messenger integrates **LiveKit**, an SFU: each client publishes one
camera/mic stream to the LiveKit server, which selectively forwards streams to participants.

**Token minting (backend).** The browser/app must not hold the LiveKit API secret, so an
authenticated user requests a short-lived token from the Sunrise backend:
`GET /v0/livekit/token?room=<room>&apikey=<key>&auth=token&secret=<session>`. The handler
(`chat/server/hdl_livekit.go`) authenticates the user (same path as file upload), then mints a
LiveKit JWT (HS256, signed with the API secret) carrying a video grant scoped to the room. Returns
`{url, token, room, identity}`, or **501** when LiveKit isn't configured (clients then use mesh).

**Backend config (environment variables):**

```
LIVEKIT_URL=wss://livekit.example.com   # or ws://localhost:7880 for local
LIVEKIT_API_KEY=devkey
LIVEKIT_API_SECRET=<32+ byte secret>
```

**Run a LiveKit server (local dev):**

```bash
docker run --rm -p 7880:7880 -p 7881:7881 -p 7882:7882/udp \
  -e LIVEKIT_KEYS="devkey: <your-secret>" \
  livekit/livekit-server --dev
```

Then set the same `devkey`/secret in the Sunrise backend env. Point `LIVEKIT_URL` at
`ws://localhost:7880`.

**Web client.** `src/lib/livekit.svelte.js` (Room connect, publish camera/mic, screen share via
`setScreenShareEnabled`, participant grid) + `LiveKitPanel`/`LiveKitTile`. Token via
`fetchLiveKitToken()`.

**Still to add for true Discord parity:** RNNoise-style noise suppression, active-speaker
indication, per-participant volume, recording/egress, and adaptive simulcast tuning (LiveKit
supports simulcast + dynacast out of the box, already enabled).

> Status: backend (`go build`) and web (`bun run build`) compile. Not yet run against a live LiveKit
> server + multiple clients. Flutter: 1:1 + screen share done; LiveKit/group + device settings are
> the next step there.
