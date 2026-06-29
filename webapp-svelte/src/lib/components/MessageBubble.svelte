<script>
  import { fileUrl, Drafty } from '../tinode.js';

  let { msg, isOwn = false, senderName = '' } = $props();

  // entitySrc resolves a Drafty entity's media source: server ref (authorized) or inline base64.
  function entitySrc(data) {
    if (!data) return '';
    if (data.ref) return fileUrl(data.ref);
    if (data.val && data.mime) return `data:${data.mime};base64,${data.val}`;
    return '';
  }

  function previewSrc(data) {
    if (data?.preref) return fileUrl(data.preref);
    if (data?.preview && data?.premime) return `data:${data.premime};base64,${data.preview}`;
    return '';
  }

  // Classify the message into a renderable shape.
  let kind = $derived.by(() => {
    if (msg?.head?.webrtc) return 'call';
    const c = msg?.content;
    if (c && typeof c === 'object' && Drafty.hasEntities?.(c)) {
      let found = 'text';
      Drafty.entities?.(c, (ent) => {
        if (found !== 'text') return;
        if (ent.tp === 'IM') found = 'image';
        else if (ent.tp === 'VD') found = (ent.data?.width && ent.data.width === ent.data?.height) ? 'videonote' : 'video';
        else if (ent.tp === 'AU') found = 'audio';
        else if (ent.tp === 'EX') found = 'file';
        else if (ent.tp === 'VC') found = 'call';
      });
      return found;
    }
    return 'text';
  });

  let firstEntity = $derived.by(() => {
    const c = msg?.content;
    if (!c || typeof c !== 'object') return null;
    let e = null;
    Drafty.entities?.(c, (ent) => { if (!e) e = ent; });
    return e;
  });

  let text = $derived.by(() => {
    const c = msg?.content;
    if (c == null) return '';
    if (typeof c === 'string') return c;
    return Drafty.toPlainText?.(c)?.trim() || '';
  });

  let time = $derived(msg?.ts ? new Date(msg.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }) : '');
  let showName = $derived(!isOwn && senderName);

  // Call message label from the webrtc head state.
  let callLabel = $derived.by(() => {
    const st = msg?.head?.webrtc;
    const dur = msg?.head?.['webrtc-duration'];
    const map = { accepted: 'Call', finished: 'Call ended', missed: 'Missed call', declined: 'Call declined', disconnected: 'Call disconnected', started: 'Calling…' };
    let label = map[st] || 'Call';
    if (dur) {
      const s = Math.round(dur / 1000);
      label += ` · ${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;
    }
    return label;
  });
</script>

<div class="message" class:own={isOwn}>
  {#if showName}<div class="sender">{senderName}</div>{/if}
  <div class="bubble-wrap">
    <div class="bubble" class:media={kind === 'image' || kind === 'video' || kind === 'videonote'}>
      {#if kind === 'image'}
        <img class="media-img" src={entitySrc(firstEntity?.data)} alt={firstEntity?.data?.name || 'image'} loading="lazy" />
      {:else if kind === 'video'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video class="media-video" src={entitySrc(firstEntity?.data)} poster={previewSrc(firstEntity?.data)} controls></video>
      {:else if kind === 'videonote'}
        <!-- svelte-ignore a11y_media_has_caption -->
        <video class="video-note" src={entitySrc(firstEntity?.data)} poster={previewSrc(firstEntity?.data)} controls loop playsinline></video>
      {:else if kind === 'audio'}
        <audio class="media-audio" src={entitySrc(firstEntity?.data)} controls></audio>
      {:else if kind === 'file'}
        <a class="file" href={entitySrc(firstEntity?.data)} target="_blank" rel="noopener" download={firstEntity?.data?.name}>
          <span class="file-icon">📎</span>
          <span class="file-meta">
            <span class="file-name">{firstEntity?.data?.name || 'File'}</span>
            {#if firstEntity?.data?.size}<span class="file-size">{(firstEntity.data.size / 1024).toFixed(0)} KB</span>{/if}
          </span>
        </a>
      {:else if kind === 'call'}
        <div class="call-chip" class:missed={msg?.head?.webrtc === 'missed' || msg?.head?.webrtc === 'declined'}>
          <span class="call-icon">📞</span><span>{callLabel}</span>
        </div>
      {/if}

      {#if text && kind !== 'call'}
        <div class="text">{text}</div>
      {/if}
      <div class="time-label">{time}</div>
    </div>
  </div>
</div>

<style>
  .message { display: flex; flex-direction: column; gap: 2px; margin-bottom: 6px; animation: fadeIn 200ms ease; }
  .message.own { align-items: flex-end; }
  .sender { font-size: 11px; color: var(--accent); font-weight: 500; margin-bottom: 2px; padding: 0 4px; }
  .bubble-wrap { max-width: 78%; }
  .bubble { padding: 10px 14px; border-radius: var(--radius-md); background: var(--bg-glass); border: 1px solid var(--border-glass); position: relative; }
  .bubble.media { padding: 6px; }
  .own .bubble { background: var(--accent-soft); border-color: rgba(139, 92, 246, 0.2); }
  .text { font-size: 14px; line-height: 1.45; color: var(--text-primary); word-wrap: break-word; white-space: pre-wrap; }
  .bubble.media .text { padding: 4px 8px 0; }
  .time-label { font-size: 11px; color: var(--text-tertiary); text-align: right; margin-top: 4px; }
  .media-img { max-width: 320px; max-height: 360px; border-radius: var(--radius-sm); display: block; cursor: pointer; }
  .media-video { max-width: 320px; border-radius: var(--radius-sm); display: block; }
  .video-note { width: 220px; height: 220px; border-radius: 50%; object-fit: cover; background: #000; }
  .media-audio { width: 260px; }
  .file { display: flex; align-items: center; gap: 10px; padding: 8px 10px; text-decoration: none; color: var(--text-primary); }
  .file-icon { font-size: 22px; }
  .file-meta { display: flex; flex-direction: column; }
  .file-name { font-size: 13px; font-weight: 500; }
  .file-size { font-size: 11px; color: var(--text-tertiary); }
  .call-chip { display: flex; align-items: center; gap: 8px; font-size: 14px; color: var(--text-primary); }
  .call-chip.missed { color: var(--danger); }
  .call-icon { font-size: 16px; }
</style>
