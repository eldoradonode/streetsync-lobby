<script>
  import { chat, findRooms, allRooms } from '../lib/chat.svelte.js';
  import RoomTile from './RoomTile.svelte';

  let q = $state('');
  let cat = $state('All');
  let searched = $state(''); // the last search sent to the server

  const everything = $derived(allRooms());
  const cats = $derived(['All', 'Official', 'Community', ...new Set(everything.map(r => r.c).filter(c => c && c !== 'Community'))]);
  const needle = $derived(q.trim().toLowerCase());
  const fits = r => {
    if (cat === 'Official' && r.k) return false;
    if (cat === 'Community' && !r.k) return false;
    if (cat !== 'All' && cat !== 'Official' && cat !== 'Community' && r.c !== cat) return false;
    if (!needle) return true;
    return r.n.toLowerCase().includes(needle) || (r.g || []).some(t => t.toLowerCase().includes(needle));
  };
  const filtering = $derived(!!needle || cat !== 'All');
  const trending = $derived(filtering ? [] : [...everything].filter(r => r.u > 0).sort((a, b) => b.h - a.h || b.u - a.u).slice(0, 3));
  // A room that is in Trending is not listed a second time on the same page. Every other room stays in the list.
  const shown = $derived(everything.filter(fits).filter(r => !trending.some(t => t.s === r.s)));
  const extra = $derived(searched && needle ? chat.found.filter(r => !everything.some(x => x.s === r.s)) : []);

  function search(e) {
    e.preventDefault();
    searched = q.trim();
    if (searched.length >= 2) findRooms(searched);
  }
</script>

<div id="roomsDir" hidden={chat.view !== 'rooms'}>
  <div class="pagehead">
    <h2>Rooms</h2>
    <p>Find a crowd. Join one, or start your own.</p>
  </div>
  <form class="row" autocomplete="off" onsubmit={search}>
    <input id="rdSearch" bind:value={q} maxlength="30" placeholder="Search by name or tag" aria-label="Search rooms">
    <button type="submit" id="rdGo">Search</button>
  </form>
  <div class="chips" id="rdChips" role="group" aria-label="Filter rooms">
    {#each cats as c (c)}<button type="button" class={'chipbtn' + (cat === c ? ' on' : '')} aria-pressed={cat === c} onclick={() => (cat = c)}>{c}</button>{/each}
  </div>

  {#if trending.length}
    <div class="sechead"><h3>Trending now</h3></div>
    <div id="rdTrending">{#each trending as r (r.s)}<RoomTile {r} />{/each}</div>
  {/if}

  <div class="sechead"><h3>{filtering ? 'Matching rooms' : trending.length ? 'More rooms' : 'All rooms'}</h3></div>
  <div id="rdList">
    {#each shown as r (r.s)}<RoomTile {r} />{/each}
    {#each extra as r (r.s)}<RoomTile {r} />{/each}
    {#if shown.length + extra.length === 0}<p class="hint empty">No rooms match. Try another word, or create the room yourself.</p>{/if}
  </div>
  <div class="row"><button type="button" id="rdCreate" onclick={() => (chat.createOpen = true)}>Create a room</button></div>
</div>
