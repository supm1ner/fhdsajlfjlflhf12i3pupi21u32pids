<script>
  let { name = '?', size = 40, src = '' } = $props();
  let initials = $derived(name.split(' ').map(w => w[0]).join('').toUpperCase().slice(0, 2) || '?');
</script>

<div class="avatar" style="width: {size}px; height: {size}px; font-size: {size * 0.4}px;">
  {#if src}
    <img {src} alt={name} />
  {:else}
    <span>{initials}</span>
  {/if}
</div>

<style>
  .avatar {
    display: flex; align-items: center; justify-content: center;
    border-radius: 50%; overflow: hidden; flex-shrink: 0;
    background: linear-gradient(135deg, var(--accent), #6d28d9);
    color: white; font-weight: 600;
  }
  .avatar img { width: 100%; height: 100%; object-fit: cover; }
</style>
