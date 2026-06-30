# Svelte messenger client

The `webapp-svelte` client built out into a working messenger on top of the existing Sunrise
backend (the Tinode-compatible Go server). It covers milestones **M2/M3/M5** of
[`ROADMAP.md`](../ROADMAP.md): real-time messaging, media + voice + round video notes
("кружки"), and WebRTC audio/video calls — all authenticated through SSO (M1).

## Architecture

```
src/lib/
  tinode.js          SDK singleton + auth (basic / oidc / token resume), 'me' topic & contacts,
                     file-upload helper, authorized file URLs, Drafty re-export
  media.js           upload + Drafty builders (image / file / voice / video note) + MIME picking
  calls.svelte.js    WebRTC call manager: invite, offer/answer/ICE over topic.videoCall(),
                     signaling via the global onInfoMessage hook, reactive call state
  oidc.js            browser PKCE login (from M1)
  stores.svelte.js   reactive app state (user, contacts, current topic, view, call)
src/views/
  Login / Register   auth screens (incl. "Sign in with SSO")
  Messenger.svelte   master-detail shell: live contact sidebar + conversation pane + call overlays
  Conversation.svelte  message feed (history + realtime), typing, composer, media, call buttons
src/lib/components/
  MessageBubble       renders Drafty: text, image, video, video note, voice, file, call entries
  TopicListItem       contact row: avatar, last message, time, unread badge, online dot
  CallPanel           active-call UI (local/remote video, mute, camera, hang-up)
  IncomingCall        incoming-call prompt (accept / decline)
  VideoNoteRecorder   round 240×240 video-note recorder (кружок)
```

## Key flows

**Contacts.** On entering the app the client subscribes to the `me` topic and builds the contact
list from `mapTopics()`; `me.onMetaSub/onSubsUpdated` keep it live. Each row shows the last message
preview, unread count and online state.

**Conversation.** Opening a chat subscribes to the topic and loads the last 24 messages via a
`MetaBuilder` query; new messages arrive through `topic.onData`. Typing runs over
`noteKeyPress`/`onPres('kp')`; `noteRead()` clears unread.

**Media.** Attachments upload through `getLargeFileHelper().upload()` (endpoint `/v0/file/u`); the
returned ref is embedded in a Drafty entity (`insertImage`/`insertVideo`/`insertAudio`/`attachFile`)
and published. Inbound media is rendered from authorized `/v0/file/s/…` URLs.

**Video notes ("кружки").** `VideoNoteRecorder` records a square (240×240) `video/webm` clip with
`MediaRecorder`, sent as a square `VD` Drafty entity and rendered as a circle.

**Calls.** An outgoing call publishes an invite (`head.webrtc='started'` + `Drafty.videoCall()`),
then exchanges SDP/ICE via `topic.videoCall(event, seq, payload)`; signaling is received through the
global `onInfoMessage` hook (`what==='call'`). Incoming calls are detected globally via
`onDataMessage` (`head.webrtc==='started'`). Standard `RTCPeerConnection` handles media; ICE servers
come from the server param `iceServers`.

## Configuration

All endpoints are configurable via **Vite env** (`VITE_*`); copy `.env.example` to `.env`.
Backend host/API key: `VITE_HOST`, `VITE_API_KEY`, `VITE_TLS`. SSO: `VITE_OIDC_ISSUER`,
`VITE_OIDC_CLIENT_ID`, `VITE_OIDC_SCOPES`, `VITE_OIDC_REDIRECT`. Defaults target local dev
(`localhost:6060` backend, `localhost:5173` SPA via `bun run dev`).

## Implemented

- **Start a new chat** — search people via the `fnd` topic (the ＋ button in the sidebar);
  tapping a result opens a P2P conversation.
- **Drafty rich text** — bold/italic/strikethrough/code/links/mentions/hashtags render via a safe
  recursive `DraftyText` renderer (link schemes restricted to http(s)/mailto/tel).
- **Reconnect** — the SDK auto-reconnects the socket; the client re-logins with the saved token and
  resumes `me` on each reconnect, and surfaces an online/connecting status.

## Known limitations / next

- Inline base64 media is not rendered (only server-ref attachments).
- 1:1 calls use the backend protocol; group calls use LiveKit (or a mesh fallback) — see CALLS.md.
- Verified by build (`bun run build`) and against the documented SDK/backend protocol; a full live
  run against a deployed stack is still pending.
