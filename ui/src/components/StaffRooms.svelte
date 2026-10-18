<script>
  import { chat, closeRoomBySlug } from '../lib/chat.svelte.js';

  let { area } = $props();
  let q = $state('');
  let kind = $state('all');
  let pending = $state('');
  const ago = ts => {
    const m = Math.max(0, Math.round((Date.now() - ts) / 60000));
    return m < 1 ? 'just now' : m < 60 ? m + ' min ago' : m < 1440 ? Math.round(m / 60) + ' h ago' : Math.round(m / 1440) + ' d ago';
  };
  const shown = $derived(chat.adm.rooms.filter(r => (kind === 'all' || (kind === 'official' ? r.k === 'official' : r.k === kind)) && (!q.trim() || r.n.toLowerCase().includes(q.trim().toLowerCase()))));
  function close(r) {
    if (pending !== r.s) { pending = r.s; return; }
    pending = '';
    closeRoomBySlug(r.s);
  }
</script>

<div class="pagehead"><h2>Rooms</h2><p>Every room, including private ones. Official rooms cannot be closed here.</p></div>
<div class="row">
  <input id="roomFilter" bind:value={q} placeholder="Filter by name" aria-label="Filter rooms" maxlength="30">
</div>
<div class="chips" role="group" aria-label="Room kind">
  {#each [['all', 'All'], ['official', 'Official'], ['public', 'Public'], ['private', 'Private']] as [k, label] (k)}
    <button type="button" class={'chipbtn' + (kind === k ? ' on' : '')} aria-pressed={kind === k} onclick={() => (kind = k)}>{label}</button>
  {/each}
</div>
<div class="ulist" id="staffRooms">
  {#each shown as r (r.s)}
    <article class="card pad ucard2" data-slug={r.s}>
      <div class="uhead">
        <div class="uid"><b>{r.n}</b><span class="tag2">{r.c}</span><span class={'tag2' + (r.k === 'private' ? ' chip' : '')}>{r.k === 'official' ? 'Official' : r.k === 'private' ? 'Private' : 'Public'}</span>
          {#if r.w}<span class="tag2 chip">Password</span>{/if}{#if r.f > 0}<span class="tag2 fee">{r.f} credits</span>{/if}</div>
        {#if r.k !== 'official'}<button type="button" class="warn small" data-act="close" onclick={() => close(r)}>{pending === r.s ? 'Tap again to close it' : 'Close room'}</button>{/if}
      </div>
      <p class="hint">{r.u} online{r.o ? ' · hosted by @' + r.o : ''}{r.la ? ' · active ' + ago(r.la) : ''}</p>
    </article>
  {/each}
  {#if shown.length === 0}<p class="hint empty">No rooms match.</p>{/if}
</div>
