# Stories & Bots — design and backend contract

The clients ship the **frontend and the client-side functional slices** of Stories
and Bots. The parts that require server support are specified here so they can be
built against the Go backend without redesign.

## What already works (client, no backend changes)

**Stickers** — fully client-side. Glyph sticker packs (`webapp/src/lib/stickers.js`);
a sticker is a message with `head.sticker='1'` (glyph in content, or an `IM` entity
for image stickers) rendered oversized and chrome-less. The data model already
supports image/animated sets — only an asset/set store is missing (below).

**Bots — the interaction primitive.** Bots drive conversations with Drafty forms
(`FM`) and buttons (`BN`), which the web client already renders and responds to
(`handleFormButtonClick` → `onFormResponse`). Plus **slash-command autocomplete**:
typing `/` in a chat surfaces commands advertised by the peer's `public.commands`
(`[{name, description}]`); selecting inserts `/command `.

**Stories — the UI + on-device slice.** Story tray (own ring + add), full-screen
viewer (segmented progress, auto-advance, keyboard nav), 24h expiry. A user's own
stories persist on-device in `localStorage` via `webapp/src/lib/stories.js`, which
is the **single seam** a backend implementation replaces.

## Stories — backend needed for cross-user distribution

A story is a short-lived media post fanned out to a user's contacts.

- **Storage/topic.** Add a per-user, append-only, TTL'd "story" feed. Simplest fit
  for Tinode: a system subtopic of `me`, e.g. `me:stories`, that a user's contacts
  are implicitly allowed to read (`R` for anyone with a P2P subscription). Each
  story is a `{data}` message with `head.story='1'`, `head.expires=<ts+24h>`, and an
  `IM`/`VD` entity.
- **Fan-out / read.** On `{sub}` to a contact you gain read access to their
  `me:stories`; `{get what=data}` returns non-expired stories. Presence of unseen
  stories is surfaced like unread counts so the tray can show rings for contacts.
- **Expiry.** Server drops messages past `head.expires` (a periodic sweep, same
  machinery as scheduled deletion).
- **Seen state.** Reuse the `{note what=read}` receipt keyed by story seq so the ring
  dims once viewed.
- **Client change.** Replace the `loadStories/addStory` body in `stories.js` with
  calls to the `me:stories` topic; the tray/viewer components stay as-is. Extend the
  tray to list contacts’ rings from their `me:stories` heads.

## Bots — backend needed for the platform

The rendering/response side is done; a bot **runtime + registry** is the server part.

- **Bot accounts.** A user flagged `fn`/`public.bot=true`; messages to it are routed
  to a bot service. Tinode already supports bot-style accounts (cf. the `Tino`
  example) — wire an auth token + a long-lived `{sub}` for the bot process.
- **Command registry.** Persist `public.commands=[{name, description}]` on the bot's
  desc so the client's slash-command menu is populated (the client already reads it).
- **Inline keyboards.** Bots reply with Drafty `FM`/`BN`; button taps post a form
  response (already handled). No client work needed.
- **Webhooks/API.** A minimal bot API (receive `{data}`, send `{pub}`) so external
  bot programs can run out-of-process. This is the bulk of the server work.

## Sticker/GIF sets — asset store

Glyph packs ship today. For image/animated (TGS/Lottie) sets and a GIF picker:

- **Set store.** An endpoint serving sticker set manifests (`{id, title, stickers:
  [{ref, mime, w, h}]}`); `StickerPicker` already renders `{ref}` stickers, so only
  the manifest fetch + an "add set" flow are new.
- **GIF picker.** A Giphy/Tenor proxy endpoint (needs an API key); GIFs already send
  and animate as image attachments — only the search/pick UI is missing.

## Status

Client slices: **done and building**. Server pieces above are scoped but not
implemented here (no running Go backend / datastore to build and test against).
