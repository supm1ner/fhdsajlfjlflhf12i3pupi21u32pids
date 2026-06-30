<script>
  import { groupCall, leaveGroupCall, toggleMute, toggleVideo, toggleScreenShare } from '../groupcall.svelte.js';
  import CallSettings from './CallSettings.svelte';
  import VideoTile from './VideoTile.svelte';

  let showSettings = $state(false);
  let count = $derived(groupCall.peers.length + 1);
</script>

<div class="group-overlay">
  <div class="topbar">
    <span class="title">Group call · {count} {count === 1 ? 'participant' : 'participants'}</span>
  </div>

  <div class="grid" style="--cols: {count <= 1 ? 1 : count <= 4 ? 2 : 3}">
    <VideoTile stream={groupCall.localStream} label="You" muted mirror={!groupCall.screenSharing} />
    {#each groupCall.peers as p (p.uid)}
      <VideoTile stream={p.stream} label={p.uid} />
    {/each}
  </div>

  {#if groupCall.error}<div class="err">{groupCall.error}</div>{/if}

  <div class="controls">
    <button class="ctrl" class:off={groupCall.muted} onclick={toggleMute} title="Mute">{groupCall.muted ? '🔇' : '🎙'}</button>
    {#if !groupCall.audioOnly}
      <button class="ctrl" class:off={groupCall.videoOff} onclick={toggleVideo} title="Camera">{groupCall.videoOff ? '📷' : '🎥'}</button>
      <button class="ctrl" class:on={groupCall.screenSharing} onclick={toggleScreenShare} title="Share screen">🖥</button>
    {/if}
    <button class="ctrl" onclick={() => showSettings = true} title="Audio & video settings">⚙️</button>
    <button class="ctrl leave" onclick={leaveGroupCall} title="Leave">📞</button>
  </div>

  {#if showSettings}
    <div class="settings-modal">
      <div class="settings-card"><CallSettings onClose={() => showSettings = false} /></div>
    </div>
  {/if}
</div>

<style>
  .group-overlay { position: absolute; inset: 0; z-index: 60; background: #0b0b12; display: flex; flex-direction: column; }
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
