<script>
  import { onMount, onDestroy } from 'svelte';
  import { pickMimeType } from '../media.js';
  import Button from './Button.svelte';

  // onFinished(blob, durationMs); onCancel()
  let { onFinished, onCancel } = $props();

  const MAX_MS = 60_000;
  const SIDE = 240;

  let videoEl = $state();
  let stream = null;
  let recorder = null;
  let chunks = [];
  let recording = $state(false);
  let elapsed = $state(0);
  let error = $state('');
  let startTs = 0;
  let timer = null;

  onMount(async () => {
    try {
      stream = await navigator.mediaDevices.getUserMedia({
        video: { width: { ideal: SIDE }, height: { ideal: SIDE }, facingMode: 'user' },
        audio: true,
      });
      if (videoEl) { videoEl.srcObject = stream; videoEl.muted = true; await videoEl.play().catch(() => {}); }
    } catch (e) {
      error = e?.message || 'Camera/microphone unavailable';
    }
  });

  onDestroy(() => cleanup());

  function cleanup() {
    if (timer) { clearInterval(timer); timer = null; }
    if (recorder && recorder.state !== 'inactive') { try { recorder.stop(); } catch { /* */ } }
    if (stream) stream.getTracks().forEach((t) => t.stop());
    stream = null;
  }

  function start() {
    if (!stream || recording) return;
    chunks = [];
    const mimeType = pickMimeType('video');
    recorder = new MediaRecorder(stream, { mimeType, videoBitsPerSecond: 1_000_000 });
    recorder.ondataavailable = (e) => { if (e.data && e.data.size) chunks.push(e.data); };
    recorder.onstop = () => {
      const duration = Date.now() - startTs;
      const blob = new Blob(chunks, { type: recorder.mimeType });
      if (timer) { clearInterval(timer); timer = null; }
      recording = false;
      if (blob.size > 0 && duration > 700) onFinished?.(blob, duration);
      else onCancel?.();
    };
    startTs = Date.now();
    recorder.start();
    recording = true;
    timer = setInterval(() => {
      elapsed = Date.now() - startTs;
      if (elapsed >= MAX_MS) stop();
    }, 100);
  }

  function stop() {
    if (recorder && recorder.state !== 'inactive') recorder.stop();
  }

  function cancel() {
    cleanup();
    onCancel?.();
  }

  let seconds = $derived((elapsed / 1000).toFixed(1));
</script>

<div class="overlay">
  <div class="recorder">
    {#if error}
      <div class="error">{error}</div>
      <Button onclick={cancel}>Close</Button>
    {:else}
      <div class="ring" class:live={recording}>
        <!-- svelte-ignore a11y_media_has_caption -->
        <video bind:this={videoEl} playsinline></video>
      </div>
      <div class="hint">{recording ? `Recording… ${seconds}s` : 'Record a round video note'}</div>
      <div class="controls">
        <button class="ctrl cancel" onclick={cancel} title="Cancel">✕</button>
        {#if recording}
          <button class="ctrl send" onclick={stop} title="Stop & send">✓</button>
        {:else}
          <button class="ctrl rec" onclick={start} title="Start recording"></button>
        {/if}
      </div>
    {/if}
  </div>
</div>

<style>
  .overlay { position: absolute; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 50; backdrop-filter: blur(6px); }
  .recorder { display: flex; flex-direction: column; align-items: center; gap: 18px; }
  .ring { width: 240px; height: 240px; border-radius: 50%; overflow: hidden; border: 4px solid var(--border-glass); background: #000; }
  .ring.live { border-color: var(--danger); box-shadow: 0 0 0 4px rgba(239,68,68,0.25); }
  .ring video { width: 100%; height: 100%; object-fit: cover; transform: scaleX(-1); }
  .hint { color: #fff; font-size: 14px; }
  .controls { display: flex; gap: 24px; align-items: center; }
  .ctrl { width: 56px; height: 56px; border-radius: 50%; border: none; cursor: pointer; font-size: 22px; color: #fff; display: flex; align-items: center; justify-content: center; }
  .ctrl.cancel { background: rgba(255,255,255,0.15); }
  .ctrl.send { background: var(--accent); }
  .ctrl.rec { background: var(--danger); width: 64px; height: 64px; border: 4px solid rgba(255,255,255,0.3); }
  .error { color: #fff; font-size: 14px; text-align: center; max-width: 240px; }
</style>
