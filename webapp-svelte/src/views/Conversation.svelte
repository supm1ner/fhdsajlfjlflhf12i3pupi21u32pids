<script>
  import { tick } from 'svelte';
  import { getClient, myUID } from '../lib/tinode.js';
  import { sendText, sendImage, sendFile, sendVoice, sendVideoNote, pickMimeType } from '../lib/media.js';
  import { startCall, callState } from '../lib/calls.svelte.js';
  import { startGroupCall } from '../lib/groupcall.svelte.js';
  import MessageBubble from '../lib/components/MessageBubble.svelte';
  import VideoNoteRecorder from '../lib/components/VideoNoteRecorder.svelte';

  let { topicName, peerName = 'Chat', online = false } = $props();

  let input = $state('');
  let messages = $state([]);
  let loading = $state(true);
  let peerTyping = $state(false);
  let sending = $state(false);
  let showVideoNote = $state(false);
  let recordingVoice = $state(false);
  let voiceElapsed = $state(0);
  let scroller;
  let fileInput;
  let imageInput;

  let typingTimer = null;
  let voiceRecorder = null;
  let voiceChunks = [];
  let voiceStart = 0;
  let voiceTimer = null;
  let lastKeyPress = 0;

  // (Re)bind to the active conversation whenever the selected topic changes.
  $effect(() => {
    const name = topicName;
    if (!name) return;
    let cancelled = false;
    messages = [];
    loading = true;
    peerTyping = false;

    const topic = getClient().getTopic(name);
    topic.onData = (msg) => { if (msg) upsert(msg); };
    topic.onPres = (pres) => {
      if (pres?.what === 'kp') flashTyping();
    };

    (async () => {
      try {
        if (!topic.isSubscribed()) {
          await topic.subscribe(topic.startMetaQuery().withLaterDesc().withLaterSub().withData(undefined, undefined, 24).build());
        } else {
          await topic.getMeta(topic.startMetaQuery().withData(undefined, undefined, 24).build());
        }
        if (cancelled) return;
        collectExisting(topic);
        topic.noteRead();
      } catch (e) {
        // ignore — empty conversation
      } finally {
        if (!cancelled) { loading = false; await scrollToBottom(); }
      }
    })();

    return () => {
      cancelled = true;
      topic.onData = undefined;
      topic.onPres = undefined;
    };
  });

  function collectExisting(topic) {
    const list = [];
    topic.messages?.((m) => { if (m && !m.head?.mcall) list.push(m); });
    messages = dedupe(list);
  }

  function dedupe(list) {
    const bySeq = new Map();
    const pending = [];
    for (const m of list) {
      if (m.seq) bySeq.set(m.seq, m);
      else pending.push(m);
    }
    return [...[...bySeq.values()].sort((a, b) => a.seq - b.seq), ...pending];
  }

  async function upsert(msg) {
    if (msg?.head?.mcall) return; // hide group-call mesh signaling from the feed
    const next = messages.filter((m) => !(m.seq && msg.seq && m.seq === msg.seq));
    next.push(msg);
    messages = dedupe(next);
    if (msg.from && msg.from !== myUID()) getClient().getTopic(topicName)?.noteRead();
    await scrollToBottom();
  }

  function flashTyping() {
    peerTyping = true;
    clearTimeout(typingTimer);
    typingTimer = setTimeout(() => { peerTyping = false; }, 3000);
  }

  async function scrollToBottom() {
    await tick();
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }

  function onInput(e) {
    input = e.target.value;
    const now = Date.now();
    if (now - lastKeyPress > 3000) {
      lastKeyPress = now;
      getClient().getTopic(topicName)?.noteKeyPress();
    }
  }

  async function send() {
    const txt = input.trim();
    if (!txt || sending) return;
    input = '';
    try { await sendText(topicName, txt); } catch (e) { console.error(e); }
  }

  async function onPickImage(e) {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    sending = true;
    try { await sendImage(topicName, file); } catch (err) { console.error(err); } finally { sending = false; }
  }

  async function onPickFile(e) {
    const file = e.target.files?.[0];
    e.target.value = '';
    if (!file) return;
    sending = true;
    try { await sendFile(topicName, file); } catch (err) { console.error(err); } finally { sending = false; }
  }

  async function onVideoNote(blob, durationMs) {
    showVideoNote = false;
    sending = true;
    try { await sendVideoNote(topicName, blob, durationMs); } catch (err) { console.error(err); } finally { sending = false; }
  }

  // --- Voice messages ---
  async function toggleVoice() {
    if (recordingVoice) { stopVoice(); return; }
    try {
      const stream = await navigator.mediaDevices.getUserMedia({ audio: true });
      voiceChunks = [];
      const mimeType = pickMimeType('audio');
      voiceRecorder = new MediaRecorder(stream, { mimeType, audioBitsPerSecond: 32_000 });
      voiceRecorder.ondataavailable = (ev) => { if (ev.data?.size) voiceChunks.push(ev.data); };
      voiceRecorder.onstop = async () => {
        const duration = Date.now() - voiceStart;
        const blob = new Blob(voiceChunks, { type: voiceRecorder.mimeType });
        stream.getTracks().forEach((t) => t.stop());
        clearInterval(voiceTimer);
        recordingVoice = false;
        if (blob.size > 0 && duration > 500) {
          sending = true;
          try { await sendVoice(topicName, blob, duration); } catch (e) { console.error(e); } finally { sending = false; }
        }
      };
      voiceStart = Date.now();
      voiceRecorder.start();
      recordingVoice = true;
      voiceElapsed = 0;
      voiceTimer = setInterval(() => { voiceElapsed = Date.now() - voiceStart; }, 200);
    } catch (e) {
      console.error('mic error', e);
    }
  }

  function stopVoice() {
    if (voiceRecorder && voiceRecorder.state !== 'inactive') voiceRecorder.stop();
  }

  function call(audioOnly) {
    if (callState.active) return;
    startCall(topicName, peerName, audioOnly);
  }

  let voiceSeconds = $derived((voiceElapsed / 1000).toFixed(1));
</script>

<div class="conversation">
  <header class="chat-header">
    <div class="chat-info">
      <div class="chat-name">{peerName}</div>
      <div class="chat-status">{peerTyping ? 'typing…' : (online ? 'online' : 'offline')}</div>
    </div>
    <div class="header-actions">
      <button class="hbtn" title="Voice call" onclick={() => call(true)}>📞</button>
      <button class="hbtn" title="Video call" onclick={() => call(false)}>🎥</button>
      <button class="hbtn" title="Group call" onclick={() => startGroupCall(topicName, false)}>👥</button>
    </div>
  </header>

  <div class="messages-area" bind:this={scroller}>
    {#if loading}
      <div class="loading">Loading…</div>
    {:else if messages.length === 0}
      <div class="empty-msg"><span>💬</span><p>No messages yet. Say hello!</p></div>
    {:else}
      {#each messages as msg (msg.seq || msg._key || msg.ts)}
        <MessageBubble {msg} isOwn={msg.from === myUID()} />
      {/each}
    {/if}
  </div>

  <div class="input-area">
    {#if recordingVoice}
      <div class="voice-recording">
        <span class="rec-dot"></span> Recording voice… {voiceSeconds}s
        <button class="vbtn stop" onclick={stopVoice}>Send ✓</button>
      </div>
    {:else}
      <div class="input-row">
        <button class="iconbtn" title="Attach file" onclick={() => fileInput.click()}>📎</button>
        <button class="iconbtn" title="Send image" onclick={() => imageInput.click()}>🖼</button>
        <input
          class="text-input"
          type="text"
          placeholder="Type a message…"
          value={input}
          oninput={onInput}
          onkeydown={(e) => e.key === 'Enter' && send()}
        />
        <button class="iconbtn" title="Record voice" onclick={toggleVoice}>🎤</button>
        <button class="iconbtn" title="Record video note" onclick={() => showVideoNote = true}>⭕</button>
        <button class="iconbtn send" title="Send" onclick={send} disabled={!input.trim()}>➤</button>
      </div>
    {/if}
    <input bind:this={fileInput} type="file" hidden onchange={onPickFile} />
    <input bind:this={imageInput} type="file" accept="image/*" hidden onchange={onPickImage} />
  </div>

  {#if showVideoNote}
    <VideoNoteRecorder onFinished={onVideoNote} onCancel={() => showVideoNote = false} />
  {/if}
</div>

<style>
  .conversation { height: 100%; display: flex; flex-direction: column; position: relative; }
  .chat-header { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border-glass); flex-shrink: 0; }
  .chat-info { flex: 1; }
  .chat-name { font-size: 15px; font-weight: 600; }
  .chat-status { font-size: 11px; color: var(--text-tertiary); }
  .header-actions { display: flex; gap: 6px; }
  .hbtn { background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: 50%; width: 38px; height: 38px; cursor: pointer; font-size: 16px; }
  .hbtn:hover { background: var(--bg-glass-hover); }
  .messages-area { flex: 1; overflow-y: auto; padding: 16px; display: flex; flex-direction: column; gap: 2px; }
  .loading, .empty-msg { display: flex; flex-direction: column; align-items: center; justify-content: center; height: 100%; gap: 8px; color: var(--text-secondary); font-size: 14px; }
  .empty-msg span { font-size: 36px; }
  .input-area { padding: 12px 16px; border-top: 1px solid var(--border-glass); flex-shrink: 0; }
  .input-row { display: flex; gap: 6px; align-items: center; }
  .text-input { flex: 1; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 10px 14px; font-size: 14px; color: var(--text-primary); }
  .text-input:focus { border-color: var(--accent); box-shadow: 0 0 0 3px var(--accent-soft); }
  .iconbtn { background: transparent; border: none; cursor: pointer; font-size: 18px; width: 36px; height: 36px; border-radius: var(--radius-sm); color: var(--text-secondary); }
  .iconbtn:hover { background: var(--bg-glass-hover); }
  .iconbtn.send { color: var(--accent); }
  .iconbtn:disabled { opacity: 0.4; cursor: default; }
  .voice-recording { display: flex; align-items: center; gap: 10px; font-size: 14px; color: var(--text-primary); }
  .rec-dot { width: 12px; height: 12px; border-radius: 50%; background: var(--danger); animation: pulse 1.2s infinite; }
  .vbtn.stop { margin-left: auto; background: var(--accent); color: #fff; border: none; border-radius: var(--radius-sm); padding: 8px 14px; cursor: pointer; }
  @keyframes pulse { 0%,100% { opacity: 1; } 50% { opacity: 0.3; } }
</style>
