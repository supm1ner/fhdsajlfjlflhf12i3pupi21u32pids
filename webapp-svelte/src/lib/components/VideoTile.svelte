<script>
  import { applySink } from '../devices.svelte.js';
  let { stream = null, label = '', muted = false, mirror = false } = $props();
  let el = $state();

  $effect(() => {
    if (el && stream && el.srcObject !== stream) {
      el.srcObject = stream;
      el.play?.().catch(() => {});
      if (!muted) applySink(el);
    }
  });
</script>

<div class="tile">
  <!-- svelte-ignore a11y_media_has_caption -->
  <video bind:this={el} playsinline autoplay {muted} class:mirror></video>
  {#if label}<span class="label">{label}</span>{/if}
</div>

<style>
  .tile { position: relative; background: #000; border-radius: var(--radius-md); overflow: hidden; aspect-ratio: 4 / 3; }
  video { width: 100%; height: 100%; object-fit: cover; }
  video.mirror { transform: scaleX(-1); }
  .label { position: absolute; left: 8px; bottom: 8px; font-size: 12px; color: #fff; background: rgba(0,0,0,0.5); padding: 2px 8px; border-radius: 8px; }
</style>
