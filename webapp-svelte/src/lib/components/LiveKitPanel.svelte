<script>
  import { liveKit, leave, toggleMute, toggleVideo, toggleScreenShare } from '../livekit.svelte.js';
  import CallSettings from './CallSettings.svelte';
  import LiveKitTile from './LiveKitTile.svelte';

  let showSettings = $state(false);
  let count = $derived(liveKit.tiles.length);
  let cols = $derived(count <= 1 ? 1 : count <= 4 ? 2 : 3);
</script>

<div class="lk-overlay">
  <div class="topbar"><span class="title">Group call (LiveKit) · {count} {count === 1 ? 'participant' : 'participants'}</span></div>

  <div class="grid" style="--cols: {cols}">
    {#each liveKit.tiles as t (t.id)}
      <LiveKitTile track={t.track} label={t.identity} isLocal={t.isLocal} />
    {/each}
  </div>

  {#if liveKit.error}<div class="err">{liveKit.error}</div>{/if}

  <div class="controls">
    <button class="ctrl" class:off={liveKit.muted} onclick={toggleMute} title="Mute">{liveKit.muted ? '🔇' : '🎙'}</button>
    {#if !liveKit.audioOnly}
      <button class="ctrl" class:off={liveKit.videoOff} onclick={toggleVideo} title="Camera">{liveKit.videoOff ? '📷' : '🎥'}</button>
      <button class="ctrl" class:on={liveKit.screenSharing} onclick={toggleScreenShare} title="Share screen">🖥</button>
    {/if}
    <button class="ctrl" onclick={() => showSettings = true} title="Audio & video settings">⚙️</button>
    <button class="ctrl leave" onclick={leave} title="Leave">📞</button>
  </div>

  {#if showSettings}
    <div class="settings-modal"><div class="settings-card"><CallSettings onClose={() => showSettings = false} /></div></div>
  {/if}
</div>

<style>
  .lk-overlay { position: absolute; inset: 0; z-index: 60; background: #0b0b12; display: flex; flex-direction: column; }
  .topbar { padding: 16px; text-align: center; }
  .title { color: #fff; font-size: 14px; font-weight: 600; }
  .grid { flex: 1; display: grid; grid-template-columns: repeat(var(--cols), 1fr); gap: 10px; padding: 0 16px 8px; overflow-y: auto; align-content: center; }
  .err { color: var(--danger); text-align: center; font-size: 12px; padding: 4px; }
  .controls { display: flex; gap: 16px; justify-content: center; padding: 20px; }
  .ctrl { width: 56px; height: 56px; border-radius: 50%; border: none; cursor: pointer; font-size: 22px; background: rgba(255,255,255,0.15); color: #fff; display: flex; align-items: center; justify-content: center; }
  .ctrl:hover { background: rgba(255,255,255,0.25); }
  .ctrl.off { background: rgba(255,255,255,0.4); }
  .ctrl.on { background: var(--accent); }
  .ctrl.leave { background: var(--danger); transform: rotate(135deg); }
  .settings-modal { position: absolute; inset: 0; display: flex; align-items: center; justify-content: center; background: rgba(0,0,0,0.4); }
  .settings-card { background: var(--bg-card); border: 1px solid var(--border-glass); border-radius: var(--radius-lg); padding: 20px; box-shadow: var(--shadow); }
</style>
