<script>
  import Avatar from './Avatar.svelte';
  import { chat, closeManage, hostUser, saveRoomSettings, refreshManage, openUser } from '../lib/chat.svelte.js';
  const m = $derived(chat.manage);
  let tab = $state('members');
  let desc = $state('');
  let seeded = '';
  $effect(() => { if (m?.ready && seeded !== m.s) { seeded = m.s; desc = m.d || ''; } });
  const when = ts => new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short' });
  const left = $derived(200 - [...desc].length);
  function act(a, h, mins) { hostUser(a, h, mins, m.s); setTimeout(refreshManage, 400); }
</script>

<section id="manage" class="mgr" hidden={chat.view !== 'manage'}>
  {#if m && m.ready}
    <div class="pagehead"><h2>{m.n}</h2><p>{m.u} online now. Room page for hosts. Staff are never affected by host actions.</p></div>
    <div class="tabs2" role="tablist">
      {#each [['members', 'Members'], ['moderation', 'Moderation'], ['reports', 'Reports (' + m.r.length + ')']] as [k, l] (k)}
        <button type="button" role="tab" id={'mgrTab-' + k} aria-selected={tab === k} class={'chipbtn' + (tab === k ? ' on' : '')} onclick={() => (tab = k)}>{l}</button>
      {/each}
    </div>

    {#if tab === 'members'}
      <div id="mgrMembers">
        {#if m.m.length === 0}<p class="hint empty">Nobody has joined yet.</p>{/if}
        {#each m.m as p (p.h)}
          <div class="mrow" data-member={p.h}>
            <button type="button" class="urow grow" onclick={() => openUser(p.h)}><Avatar handle={p.h} size={32} /><b>@{p.h}</b><small>{p.l ? 'left' : 'joined'} {when(p.j)}</small></button>
            <button type="button" class="warn small" data-act="remove" onclick={() => act('remove', p.h)}>Remove</button>
          </div>
        {/each}
      </div>
    {:else if tab === 'moderation'}
      <div id="mgrMod">
        <p class="hint">Mute stops someone from writing for a while. Kick sends them out for a while. Remove takes away their place in the room.</p>
        {#each m.m as p (p.h)}
          <div class="mrow" data-member={p.h}>
            <span class="grow">@{p.h}{#if p.mu} <span class="tag2 fee">muted {p.mu} min</span>{/if}{#if p.ki} <span class="tag2 fee">kicked {p.ki} min</span>{/if}</span>
            <button type="button" class="ghost small" data-act="mute" onclick={() => act('mute', p.h, 10)}>Mute 10 min</button>
            <button type="button" class="ghost small" data-act="unmute" onclick={() => act('unmute', p.h, 0)}>Unmute</button>
            <button type="button" class="warn small" data-act="kick" onclick={() => act('kick', p.h, 10)}>Kick 10 min</button>
          </div>
        {/each}
        <h3>Room details</h3>
        <label for="mgrDesc">Description</label>
        <textarea id="mgrDesc" bind:value={desc} rows="2" maxlength="200"></textarea>
        <small class="hint">{left} characters left</small>
        <div class="row"><button type="button" id="mgrSave" disabled={left < 0} onclick={() => { saveRoomSettings({ description: desc, room: m.s }); setTimeout(refreshManage, 400); }}>Save</button></div>
      </div>
    {:else}
      <div id="mgrReports">
        <p class="hint">What people reported about this room. Reporter names are only visible to staff.</p>
        {#if m.r.length === 0}<p class="hint empty">No reports.</p>{/if}
        {#each m.r as r}<div class="mrow"><span class="tag2 fee">{r.c}</span><span class="grow">{r.d || 'No details'}</span><small>{r.s} · {when(r.ts)}</small></div>{/each}
      </div>
    {/if}
    <div class="row"><button type="button" class="ghost" onclick={closeManage}>Back to rooms</button></div>
  {:else}
    <p class="hint">Loading.</p>
  {/if}
</section>
