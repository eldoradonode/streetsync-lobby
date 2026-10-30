<script>
  import Avatar from './Avatar.svelte';
  import { chat, closeMembers, moreMembers, openUser } from '../lib/chat.svelte.js';
  import { groupMembers, STATUS } from '../lib/rooms20.js';

  const g = $derived(groupMembers(chat.members.list));
  const m = $derived(chat.members);
  const sections = $derived([
    { id: 'on', title: 'Online', rows: g.on },
    { id: 'hid', title: 'Status hidden', rows: g.hid },
    { id: 'off', title: 'Offline', rows: g.off },
  ].filter(s => s.rows.length));

  function key(e) { if (e.key === 'Escape') closeMembers(); }
</script>

<svelte:window onkeydown={m.open ? key : null} />

{#if m.open}
  <div id="members" class="sheet" role="dialog" aria-modal="false" aria-label="People in this room">
    <div class="sheethead">
      <h3>People</h3>
      <span class="mcount" aria-live="polite">{m.on} online &middot; {m.tot} {m.tot === 1 ? 'member' : 'members'}</span>
      <button type="button" class="ghost" id="membersClose" aria-label="Close the people list" onclick={closeMembers}>&#10005;</button>
    </div>
    <div class="mbody">
      {#if m.err}<p class="hint empty" role="alert">{m.err}</p>{/if}
      {#if m.busy && !m.list.length}<p class="hint empty">Loading&hellip;</p>{/if}
      {#each sections as s (s.id)}
        <div class="msec" data-sec={s.id}>{s.title} ({s.rows.length})</div>
        <ul class="mlist">
          {#each s.rows as p (p.h)}
            <li>
              <button type="button" class="mrow" data-handle={p.h} data-status={p.s} onclick={() => openUser(p.h)}>
                <Avatar handle={p.h} size={32} />
                <span class="mname">@{p.h}{#if p.x}<span class="bd host">HOST</span>{/if}{#if p.o}<span class={'bd r-' + p.o}>{p.o === 'admin' ? 'ADMIN' : 'MOD'}</span>{/if}</span>
                <span class={'mstat st-' + p.s}><span class="mmark" aria-hidden="true">{STATUS[p.s].mark}</span> {STATUS[p.s].label}</span>
              </button>
            </li>
          {/each}
        </ul>
      {/each}
      {#if !m.busy && !m.list.length && !m.err}<p class="hint empty">No one to show yet.</p>{/if}
      {#if m.more}<button type="button" class="ghost" id="membersMore" disabled={m.busy} onclick={moreMembers}>{m.busy ? 'Loading\u2026' : 'Show more'}</button>{/if}
    </div>
  </div>
{/if}
