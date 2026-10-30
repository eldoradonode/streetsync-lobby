<script>
  import { chat, chooseRoom, isMod, leaveMembership, openManage, isFavourite, toggleFavourite } from '../lib/chat.svelte.js';
  import { TIERS, tierOf } from '../lib/rank.js';
  import { colorFor } from '../lib/avatar.js';
  import { roomIcon } from '../lib/roomicons.js';

  let { r } = $props();
  const locked = $derived((r.t || 0) > chat.tier && !isMod());
  const mineRoom = $derived(!!r.k && r.o === chat.me);
  const canLeave = $derived(!!r.k && !!r.j && !mineRoom);
  const canManage = $derived(!!r.k && (mineRoom || isMod()));
  let sure = $state(false);
  function leave() { if (!sure) { sure = true; setTimeout(() => (sure = false), 4000); return; } sure = false; leaveMembership(r.s); }
</script>

<div class="roomwrap">
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
    {#if r.d}<span class="descr">{r.d}</span>{/if}
    <span class="tags">
      {#if r.k}
        <span class="tag2 host">{r.o === chat.me ? 'Yours' : '@' + r.o}</span>
        {#if r.j && !mineRoom}<span class="tag2 chip" data-joined>Joined</span>{:else if r.y && !mineRoom}<span class="tag2 chip">Left, free to rejoin</span>{/if}
        {#if r.k === 'private'}<span class="tag2 chip">Private</span>{/if}
        {#if r.w}<span class="tag2 chip">Password</span>{/if}
        {#if r.f > 0}<span class="tag2 chip fee">{r.f} credits</span>{/if}
      {/if}
      {#each r.g || [] as tg}<span class="tag2">{tg}</span>{/each}
    </span>
  </span>
  <span class={'joinpill' + (r.j ? ' on' : '') + (chat.joining === r.s ? ' busy' : '')} data-state={r.j ? 'joined' : 'join'} aria-live="polite">{locked ? 'Locked' : chat.joining === r.s ? 'Joining\u2026' : r.j ? '\u2713 Joined' : r.y && r.k ? 'Rejoin' : 'Join'}</span>
</button>
<button type="button" class={'favbtn' + (isFavourite(r.s) ? ' on' : '')} aria-pressed={isFavourite(r.s)} aria-label={(isFavourite(r.s) ? 'Remove ' : 'Add ') + r.n + (isFavourite(r.s) ? ' from favourites' : ' to favourites')} data-fav={r.s} onclick={() => toggleFavourite(r.s)}>{isFavourite(r.s) ? '\u2605' : '\u2606'}</button>
{#if canLeave || canManage}
  <div class="roomacts">
    {#if canManage}<button type="button" class="ghost small" data-act="manage" onclick={() => openManage(r.s)}>Manage</button>{/if}
    {#if canLeave}<button type="button" class="ghost small" data-act="leave" onclick={leave}>{sure ? 'Tap again to leave' : 'Leave room'}</button>{/if}
  </div>
{/if}
</div>
