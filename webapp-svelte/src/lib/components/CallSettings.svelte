<script>
  import { onMount } from 'svelte';
  import {
    deviceState, refreshDevices,
    setAudioInput, setAudioOutput, setVideoInput, setProcessing,
  } from '../devices.svelte.js';

  let { onClose } = $props();
  let ready = $state(false);

  onMount(async () => {
    // Labels are only exposed after a media permission grant.
    try { await navigator.mediaDevices.getUserMedia({ audio: true, video: true }).then(s => s.getTracks().forEach(t => t.stop())); } catch { /* */ }
    await refreshDevices();
    ready = true;
  });

  const supportsSink = typeof HTMLMediaElement !== 'undefined' && 'setSinkId' in HTMLMediaElement.prototype;
</script>

<div class="settings">
  <div class="head">
    <h3>Audio & video</h3>
    {#if onClose}<button class="close" onclick={onClose}>✕</button>{/if}
  </div>

  {#if !ready}
    <p class="muted">Detecting devices…</p>
  {:else}
    <label class="field">
      <span>Microphone</span>
      <select value={deviceState.audioInput} onchange={(e) => setAudioInput(e.target.value)}>
        <option value="">Default</option>
        {#each deviceState.devices.audioinput as d}<option value={d.deviceId}>{d.label}</option>{/each}
      </select>
    </label>

    {#if supportsSink}
      <label class="field">
        <span>Speaker</span>
        <select value={deviceState.audioOutput} onchange={(e) => setAudioOutput(e.target.value)}>
          <option value="">Default</option>
          {#each deviceState.devices.audiooutput as d}<option value={d.deviceId}>{d.label}</option>{/each}
        </select>
      </label>
    {/if}

    <label class="field">
      <span>Camera</span>
      <select value={deviceState.videoInput} onchange={(e) => setVideoInput(e.target.value)}>
        <option value="">Default</option>
        {#each deviceState.devices.videoinput as d}<option value={d.deviceId}>{d.label}</option>{/each}
      </select>
    </label>

    <div class="toggles">
      <label><input type="checkbox" checked={deviceState.noiseSuppression} onchange={(e) => setProcessing({ noiseSuppression: e.target.checked })} /> Noise suppression</label>
      <label><input type="checkbox" checked={deviceState.echoCancellation} onchange={(e) => setProcessing({ echoCancellation: e.target.checked })} /> Echo cancellation</label>
      <label><input type="checkbox" checked={deviceState.autoGainControl} onchange={(e) => setProcessing({ autoGainControl: e.target.checked })} /> Auto gain</label>
    </div>
  {/if}
</div>

<style>
  .settings { display: flex; flex-direction: column; gap: 14px; min-width: 280px; }
  .head { display: flex; align-items: center; justify-content: space-between; }
  .head h3 { font-size: 15px; font-weight: 600; color: var(--text-primary); }
  .close { background: none; border: none; font-size: 16px; color: var(--text-secondary); cursor: pointer; }
  .muted { font-size: 13px; color: var(--text-secondary); }
  .field { display: flex; flex-direction: column; gap: 6px; }
  .field span { font-size: 12px; color: var(--text-secondary); }
  .field select { background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-sm); padding: 9px 12px; font-size: 13px; color: var(--text-primary); }
  .toggles { display: flex; flex-direction: column; gap: 8px; margin-top: 4px; }
  .toggles label { display: flex; align-items: center; gap: 8px; font-size: 13px; color: var(--text-primary); cursor: pointer; }
  .toggles input { accent-color: var(--accent); }
</style>
