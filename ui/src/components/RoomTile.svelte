<script>
  import { chat, chooseRoom, isMod } from '../lib/chat.svelte.js';
  import { TIERS, tierOf } from '../lib/rank.js';
  import { colorFor } from '../lib/avatar.js';
  import { roomIcon } from '../lib/roomicons.js';

  let { r } = $props();
  const locked = $derived((r.t || 0) > chat.tier && !isMod());
</script>

<button type="button" class={'room' + (locked ? ' locked' : '')} data-slug={r.s} onclick={() => chooseRoom(r)}>
  <span class="icon" style:--ic={colorFor(r.c)} aria-hidden="true">{roomIcon(r)}</span>
  <span class="info">
    <span class="row1">
      <span class="nm">{r.n}</span>
      <span class="stats">
        {#if locked}<span class="lock">Needs {TIERS[tierOf(r.t)]}</span>{/if}
        <span class="on">{r.u} online</span>
        <span class="flames" aria-label={'Activity ' + r.h + ' of 5'}>{#each [0, 1, 2, 3, 4] as i}<span class={i >= r.h ? 'off' : ''}>&#9632;</span>{/each}</span>
      </span>
    </span>
    <span class="tags">
      {#if r.k}
        <span class="tag2 host">{r.o === chat.me ? 'Yours' : '@' + r.o}</span>
        {#if r.k === 'private'}<span class="tag2 chip">Private</span>{/if}
        {#if r.w}<span class="tag2 chip">Password</span>{/if}
        {#if r.f > 0}<span class="tag2 chip fee">{r.f} credits</span>{/if}
      {/if}
      {#each r.g || [] as tg}<span class="tag2">{tg}</span>{/each}
    </span>
  </span>
  <span class="joinpill" aria-hidden="true">{locked ? 'Locked' : 'Join'}</span>
</button>
