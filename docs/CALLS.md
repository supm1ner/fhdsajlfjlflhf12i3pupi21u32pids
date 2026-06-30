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

**Tradeoffs / "better than Discord" path.** Mesh is simple and serverless but each client
uploads its stream to every peer, so it scales to ~4-6 participants. For large rooms the real
solution is an **SFU** (LiveKit, mediasoup, Janus): each client sends one stream up and the SFU
fans it out. Other Discord-grade features to add: RNNoise-style noise suppression, active-speaker
detection, per-participant volume, and a dedicated signaling verb so signaling doesn't ride on
stored messages. These are tracked in ROADMAP M5.

> Status: implemented in the **web** client and verified by `bun run build`. Not yet run against a
> live multi-client stack. Flutter has 1:1 + screen share; group calls and device settings are the
> next step there.
