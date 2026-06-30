<script>
  import { Drafty } from '../tinode.js';
  import Self from './DraftyText.svelte';

  // Either `content` (a Drafty doc/string, top-level) or `node` (recursion).
  let { content = undefined, node = undefined } = $props();

  // Build a render tree from Drafty: each node is { type, data, children:[node|string] }.
  function build(c) {
    if (c == null) return { type: '', children: [] };
    if (typeof c === 'string') return { type: '', children: [c] };
    try {
      const root = Drafty.format(c, (type, data, values) => ({ type: type || '', data, children: values || [] }), null);
      return root && typeof root === 'object' ? root : { type: '', children: [Drafty.toPlainText?.(c) || ''] };
    } catch {
      return { type: '', children: [Drafty.toPlainText?.(c) || ''] };
    }
  }

  // Only allow safe link schemes.
  function safeHref(url) {
    if (typeof url !== 'string') return null;
    return /^(https?:|mailto:|tel:)/i.test(url) ? url : null;
  }

  let n = $derived(node ?? build(content));
  let kids = $derived(Array.isArray(n.children) ? n.children : (n.children != null ? [n.children] : []));
  let href = $derived(n.type === 'LN' ? safeHref(n.data?.url) : null);
</script>

{#snippet children()}{#each kids as c}{#if typeof c === 'string'}{c}{:else}<Self node={c} />{/if}{/each}{/snippet}

{#if n.type === 'ST'}<strong>{@render children()}</strong>
{:else if n.type === 'EM'}<em>{@render children()}</em>
{:else if n.type === 'DL'}<s>{@render children()}</s>
{:else if n.type === 'CO'}<code>{@render children()}</code>
{:else if n.type === 'BR'}<br />
{:else if n.type === 'LN'}{#if href}<a {href} target="_blank" rel="noopener noreferrer">{@render children()}</a>{:else}{@render children()}{/if}
{:else if n.type === 'MN'}<span class="mention">{@render children()}</span>
{:else if n.type === 'HT'}<span class="hashtag">{@render children()}</span>
{:else}{@render children()}{/if}

<style>
  code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; background: var(--bg-glass); padding: 1px 5px; border-radius: 5px; font-size: 0.92em; }
  a { color: var(--accent); text-decoration: underline; }
  .mention { color: var(--accent); font-weight: 500; }
  .hashtag { color: var(--accent); }
</style>
