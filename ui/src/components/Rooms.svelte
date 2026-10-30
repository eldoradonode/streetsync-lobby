<script>
  import Avatar from './Avatar.svelte';
  import { chat, liveSearch, allRooms, openUser, setDirSort } from '../lib/chat.svelte.js';
  import { SORTS } from '../lib/rooms20.js';
  import RoomTile from './RoomTile.svelte';

  let q = $state('');
  let cat = $state('All');
  let tab = $state('rooms'); // rooms or people
  $effect(() => { if (chat.roomsCat) { cat = chat.roomsCat; tab = 'rooms'; chat.roomsCat = ''; } });

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
  const trending = $derived(filtering || chat.dirSort !== 'active' ? [] : [...everything].filter(r => r.u > 0).sort((a, b) => b.h - a.h || b.u - a.u).slice(0, 3));
  // A room that is in Trending is not listed a second time on the same page. Every other room stays in the list.
  const shown = $derived(everything.filter(fits).filter(r => !trending.some(t => t.s === r.s)));
  const extra = $derived(needle ? chat.found.filter(r => !everything.some(x => x.s === r.s)) : []);
  const roomCount = $derived(shown.length + extra.length);
  const peopleQuery = $derived(needle.replace(/^@/, '').length);

  // Results follow the typing (after a 250 ms pause). Enter, or the Search button, asks at once.
  const typed = () => liveSearch(q);
  function search(e) { e.preventDefault(); liveSearch(q, true); }
</script>

<div id="roomsDir" hidden={chat.view !== 'rooms'}>
  <div class="pagehead">
    <h2>Rooms</h2>
    <p>Find a crowd. Join one, or start your own.</p>
  </div>
  <form class="row" autocomplete="off" onsubmit={search}>
    <input id="rdSearch" type="text" inputmode="search" enterkeyhint="search" autocapitalize="off" autocorrect="off" spellcheck="false" bind:value={q} oninput={typed} maxlength="30" placeholder="Search rooms or people" aria-label="Search rooms or people">
    <button type="submit" id="rdGo">Search</button>
  </form>
  <div class="tabs2" role="tablist" aria-label="Search results">
    <button type="button" role="tab" id="tabRoomsRes" aria-selected={tab === 'rooms'} class={'chipbtn' + (tab === 'rooms' ? ' on' : '')} onclick={() => (tab = 'rooms')}>Rooms</button>
    <button type="button" role="tab" id="tabUsersRes" aria-selected={tab === 'people'} class={'chipbtn' + (tab === 'people' ? ' on' : '')} onclick={() => (tab = 'people')}>People{#if chat.ufound.q} ({chat.ufound.r.length}){/if}</button>
  </div>
  {#if tab === 'people'}
    <div id="userResults" class="ulist2">
      {#if !chat.ufound.q}<p class="hint empty" id="peopleHint">Type at least 2 letters of a handle.</p>{/if}
      {#if chat.ufound.busy}<p class="hint empty" id="peopleBusy">Searching...</p>{/if}
      {#each chat.ufound.r as u (u.h)}
        <button type="button" class="urow" data-handle={u.h} onclick={() => openUser(u.h)}>
          <Avatar handle={u.h} size={36} />
          <b>@{u.h}</b>
          {#if u.o}<span class={'bd r-' + u.o}>{u.o === 'admin' ? 'ADMIN' : 'MOD'}</span>{/if}
        </button>
      {/each}
      {#if chat.ufound.q && !chat.ufound.busy && chat.ufound.r.length === 0}<p class="hint empty">No one matches "{chat.ufound.q}".</p>{/if}
    </div>
  {/if}
  <div class="chips" id="rdChips" hidden={tab !== 'rooms'} role="group" aria-label="Filter rooms">
    {#each cats as c (c)}<button type="button" class={'chipbtn' + (cat === c ? ' on' : '')} aria-pressed={cat === c} onclick={() => (cat = c)}>{c}</button>{/each}
  </div>

  {#if tab === 'rooms' && !filtering && chat.favRooms.length}
    <div class="sechead"><h3>Favourites</h3></div>
    <div id="rdFavs">{#each chat.favRooms as r (r.s)}<RoomTile {r} />{/each}</div>
  {/if}
  {#if tab === 'rooms' && !filtering && chat.recentRooms.length}
    <div class="sechead"><h3>Recently visited</h3></div>
    <div id="rdRecent">{#each chat.recentRooms as r (r.s)}<RoomTile {r} />{/each}</div>
  {/if}

  {#if tab === 'rooms' && trending.length}
    <div class="sechead"><h3>Trending now</h3></div>
    <div id="rdTrending">{#each trending as r (r.s)}<RoomTile {r} />{/each}</div>
  {/if}

  {#if tab === 'rooms'}
  <div class="chips" id="rdSort" role="group" aria-label="Sort rooms"><span class="sortlabel">Sort</span>{#each SORTS as s (s.id)}<button type="button" class={'chipbtn' + (chat.dirSort === s.id ? ' on' : '')} aria-pressed={chat.dirSort === s.id} data-sort={s.id} onclick={() => setDirSort(s.id)}>{s.label}</button>{/each}</div>
  <div class="sechead"><h3>{filtering ? 'Matching rooms' + (needle ? ' (' + roomCount + ')' : '') : trending.length ? 'More rooms' : 'All rooms'}</h3></div>
  <div id="rdList">
    {#each shown as r (r.s)}<RoomTile {r} />{/each}
    {#each extra as r (r.s)}<RoomTile {r} />{/each}
    {#if shown.length + extra.length === 0}<p class="hint empty">No rooms match{needle ? ' "' + q.trim() + '"' : ''}.{peopleQuery >= 2 && chat.ufound.r.length ? ' Try the People tab.' : ' Try another word, or create the room yourself.'}</p>{/if}
  </div>
  {/if}
  <div class="row"><button type="button" id="rdCreate" onclick={() => (chat.createOpen = true)}>Create a room</button></div>
</div>
