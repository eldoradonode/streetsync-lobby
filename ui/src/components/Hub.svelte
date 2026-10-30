<script>
  import { chat, findRooms, lookupCode, allRooms, go } from '../lib/chat.svelte.js';
  import JoinPanel from './JoinPanel.svelte';
  import RoomTile from './RoomTile.svelte';

  let tool = $state(''); // '', 'find' or 'code'
  let findEl, codeEl;

  const official = $derived(chat.rooms.filter(r => !r.k));
  const communityAll = $derived(allRooms().filter(r => r.k));
  const community = $derived(communityAll.slice(0, 8)); // the Rooms page lists the rest

  // Featured: the busiest rooms right now, official or community. Categories: one tile for each, with how many rooms.
  const featured = $derived([...chat.rooms.filter(r => !r.k), ...allRooms().filter(r => r.k && !r.w && r.k !== 'private')]
    .filter(r => r.u > 0).sort((a, b) => b.h - a.h || b.u - a.u).slice(0, 3));
  const toggle = name => { if (name === 'create') chat.createOpen = !chat.createOpen; else tool = tool === name ? '' : name; };
  function find(e) { e.preventDefault(); findRooms(findEl.value); }
  function useCode(e) {
    e.preventDefault();
    if (lookupCode(codeEl.value)) { codeEl.value = ''; tool = ''; }
  }
</script>


<div id="hub" hidden={chat.view !== 'hub'}>
  <div class="pagehead">
    <h2>Hello, @{chat.me}</h2>
    <p>Pick a room and start talking.</p>
  </div>
  <JoinPanel />
  <div class="sechead"><h3>Official rooms</h3></div>
  <div id="roomList">
    {#each official as r (r.s)}<RoomTile {r} />{/each}
  </div>
  {#if featured.length}
    <div class="sechead"><h3>Featured</h3></div>
    <div id="hubFeatured" class="featured">{#each featured as r (r.s)}<RoomTile {r} />{/each}</div>
  {/if}

  <div id="communityHead">
    <h2>Community rooms</h2>
    <div class="row tools">
      <button type="button" class="small" id="toolCreate" onclick={() => toggle('create')}>Create</button>
      <button type="button" class="ghost small" id="toolFind" onclick={() => toggle('find')}>Find</button>
      <button type="button" class="ghost small" id="toolCode" onclick={() => toggle('code')}>Use a code</button>
    </div>
  </div>
  {#if tool === 'find'}
    <form id="findForm" class="row" autocomplete="off" onsubmit={find}>
      <input id="findInput" bind:this={findEl} maxlength="30" placeholder="Search by name or tag" aria-label="Search rooms">
      <button type="submit">Search</button>
    </form>
    <div id="foundList">
      {#each chat.found as r (r.s)}<RoomTile {r} />{/each}
    </div>
  {/if}
  {#if tool === 'code'}
    <form id="codeForm" class="row" autocomplete="off" onsubmit={useCode}>
      <input id="codeInput" bind:this={codeEl} maxlength="8" placeholder="8 letter invite code" autocapitalize="characters" aria-label="Invite code">
      <button type="submit">Find room</button>
    </form>
  {/if}
  <div id="communityList">
    {#each community as r (r.s)}<RoomTile {r} />{/each}
    {#if community.length === 0}
      <p class="hint empty">No community rooms yet. Create the first one.</p>
    {/if}
  </div>
  {#if communityAll.length > community.length}
    <div class="row"><button type="button" class="ghost" id="hubAllRooms" onclick={() => go('rooms')}>See all {communityAll.length} community rooms</button></div>
  {/if}
</div>
