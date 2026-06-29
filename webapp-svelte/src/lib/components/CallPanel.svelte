<script>
  import { callState, hangup, toggleMute, toggleVideo } from '../calls.svelte.js';

  let localEl = $state();
  let remoteEl = $state();

  // Bind MediaStreams to the video elements reactively.
  $effect(() => {
    if (localEl && callState.localStream && localEl.srcObject !== callState.localStream) {
      localEl.srcObject = callState.localStream;
      localEl.play?.().catch(() => {});
    }
  });
  $effect(() => {
    if (remoteEl && callState.remoteStream && remoteEl.srcObject !== callState.remoteStream) {
      remoteEl.srcObject = callState.remoteStream;
      remoteEl.play?.().catch(() => {});
    }
  });

  let statusText = $derived.by(() => {
    switch (callState.status) {
      case 'dialing': return 'Calling…';
      case 'ringing': return 'Ringing…';
      case 'connecting': return 'Connecting…';
      case 'active': return 'Connected';
      default: return '';
    }
  });
</script>

<div class="call-overlay">
  <div class="stage">
    {#if !callState.audioOnly}
      <!-- svelte-ignore a11y_media_has_caption -->
      <video class="remote" bind:this={remoteEl} playsinline autoplay></video>
      <!-- svelte-ignore a11y_media_has_caption -->
      <video class="local" bind:this={localEl} playsinline autoplay muted></video>
    {:else}
      <!-- Audio-only: hidden media elements still carry the streams -->
      <!-- svelte-ignore a11y_media_has_caption -->
      <video class="hidden" bind:this={remoteEl} playsinline autoplay></video>
      <!-- svelte-ignore a11y_media_has_caption -->
      <video class="hidden" bind:this={localEl} playsinline autoplay muted></video>
      <div class="audio-card">
        <div class="avatar-big">{(callState.peerName || '?')[0]}</div>
        <div class="peer">{callState.peerName}</div>
      </div>
    {/if}

    <div class="status-bar">
      <span class="peer-name">{callState.peerName}</span>
      <span class="status">{statusText}</span>
      {#if callState.error}<span class="err">{callState.error}</span>{/if}
    </div>

    <div class="controls">
      <button class="ctrl" class:off={callState.muted} onclick={toggleMute} title="Mute">
        {callState.muted ? '🔇' : '🎙'}
      </button>
      {#if !callState.audioOnly}
        <button class="ctrl" class:off={callState.videoOff} onclick={toggleVideo} title="Camera">
          {callState.videoOff ? '📷' : '🎥'}
        </button>
      {/if}
      <button class="ctrl hangup" onclick={hangup} title="Hang up">📞</button>
    </div>
  </div>
</div>

<style>
  .call-overlay { position: absolute; inset: 0; z-index: 60; background: #0b0b12; display: flex; align-items: center; justify-content: center; }
  .stage { position: relative; width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; }
  .remote { width: 100%; height: 100%; object-fit: cover; background: #000; }
  .local { position: absolute; right: 20px; bottom: 96px; width: 180px; height: 135px; object-fit: cover; border-radius: var(--radius-md); border: 2px solid rgba(255,255,255,0.2); transform: scaleX(-1); background: #000; }
  .hidden { display: none; }
  .audio-card { display: flex; flex-direction: column; align-items: center; gap: 14px; }
  .avatar-big { width: 120px; height: 120px; border-radius: 50%; background: linear-gradient(135deg, var(--accent), #6d28d9); color: #fff; font-size: 48px; display: flex; align-items: center; justify-content: center; text-transform: uppercase; }
  .peer { color: #fff; font-size: 20px; }
  .status-bar { position: absolute; top: 24px; left: 0; right: 0; display: flex; flex-direction: column; align-items: center; gap: 4px; color: #fff; }
  .peer-name { font-size: 18px; font-weight: 600; }
  .status { font-size: 13px; opacity: 0.8; }
  .err { font-size: 12px; color: var(--danger); }
  .controls { position: absolute; bottom: 28px; left: 0; right: 0; display: flex; gap: 18px; justify-content: center; }
  .ctrl { width: 60px; height: 60px; border-radius: 50%; border: none; cursor: pointer; font-size: 24px; background: rgba(255,255,255,0.15); color: #fff; display: flex; align-items: center; justify-content: center; transition: var(--transition); }
  .ctrl:hover { background: rgba(255,255,255,0.25); }
  .ctrl.off { background: rgba(255,255,255,0.4); }
  .ctrl.hangup { background: var(--danger); transform: rotate(135deg); }
</style>
