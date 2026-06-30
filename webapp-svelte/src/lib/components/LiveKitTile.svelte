<script>
  let { track = null, label = '', isLocal = false } = $props();
  let el = $state();

  // Attach/detach the LiveKit video track to the <video> element.
  $effect(() => {
    const t = track;
    const node = el;
    if (t && node) {
      t.attach(node);
      return () => { try { t.detach(node); } catch { /* */ } };
    }
  });
</script>

<div class="tile">
  {#if track}
    <!-- svelte-ignore a11y_media_has_caption -->
    <video bind:this={el} playsinline autoplay muted={isLocal} class:mirror={isLocal}></video>
  {:else}
    <div class="placeholder">{(label || '?')[0]}</div>
  {/if}
  {#if label}<span class="label">{label}</span>{/if}
</div>

<style>
  .tile { position: relative; background: #000; border-radius: var(--radius-md); overflow: hidden; aspect-ratio: 4 / 3; }
  video { width: 100%; height: 100%; object-fit: cover; }
  video.mirror { transform: scaleX(-1); }
  .placeholder { width: 100%; height: 100%; display: flex; align-items: center; justify-content: center; font-size: 40px; color: #fff; background: linear-gradient(135deg, var(--accent), #6d28d9); text-transform: uppercase; }
  .label { position: absolute; left: 8px; bottom: 8px; font-size: 12px; color: #fff; background: rgba(0,0,0,0.5); padding: 2px 8px; border-radius: 8px; }
</style>
