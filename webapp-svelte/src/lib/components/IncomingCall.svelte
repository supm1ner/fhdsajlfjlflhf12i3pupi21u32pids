<script>
  import { callState, acceptCall, declineCall } from '../calls.svelte.js';
</script>

<div class="incoming-overlay">
  <div class="card">
    <div class="avatar">{(callState.peerName || '?')[0]}</div>
    <div class="name">{callState.peerName}</div>
    <div class="sub">Incoming {callState.audioOnly ? 'voice' : 'video'} call…</div>
    <div class="actions">
      <button class="btn decline" onclick={declineCall} title="Decline">📞</button>
      <button class="btn accept" onclick={acceptCall} title="Accept">📞</button>
    </div>
  </div>
</div>

<style>
  .incoming-overlay { position: absolute; inset: 0; z-index: 70; background: rgba(0,0,0,0.65); display: flex; align-items: center; justify-content: center; backdrop-filter: blur(6px); }
  .card { display: flex; flex-direction: column; align-items: center; gap: 12px; padding: 32px 40px; background: var(--bg-glass); border: 1px solid var(--border-glass); border-radius: var(--radius-lg); animation: fadeIn 200ms ease; }
  .avatar { width: 96px; height: 96px; border-radius: 50%; background: linear-gradient(135deg, var(--accent), #6d28d9); color: #fff; font-size: 40px; display: flex; align-items: center; justify-content: center; text-transform: uppercase; animation: pulse 1.5s infinite; }
  .name { font-size: 20px; font-weight: 600; color: var(--text-primary); }
  .sub { font-size: 13px; color: var(--text-secondary); }
  .actions { display: flex; gap: 40px; margin-top: 12px; }
  .btn { width: 60px; height: 60px; border-radius: 50%; border: none; cursor: pointer; font-size: 26px; color: #fff; }
  .btn.accept { background: var(--success, #22c55e); }
  .btn.decline { background: var(--danger); transform: rotate(135deg); }
  @keyframes pulse { 0%,100% { box-shadow: 0 0 0 0 rgba(139,92,246,0.5); } 50% { box-shadow: 0 0 0 16px rgba(139,92,246,0); } }
</style>
