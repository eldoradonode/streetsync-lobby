<script>
  import Avatar from './Avatar.svelte';
  import { chat, findUsers, staffAct, grantCredits, banUser, unbanUser } from '../lib/chat.svelte.js';
  import { TIERS, tierOf } from '../lib/rank.js';

  let { area } = $props();
  let q = $state(chat.adm.q);
  let pending = $state(''); // "handle:action" waiting for a second tap
  let mins = $state({}); // chosen minutes per handle
  let give = $state({});
  const admin = $derived(chat.role === 'admin');
  const joined = ts => new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short', year: 'numeric' });
  const seen = ts => new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short' });
  const mine = u => u.h.toLowerCase() === (chat.me || '').toLowerCase();
  // Nobody acts on themselves or on an admin. Only an admin acts on a moderator.
  const reach = u => !mine(u) && (u.o === 'user' || (u.o === 'mod' && admin));

  function search(e) { e.preventDefault(); findUsers(q); }
  function act(a, u, m) {
    const key = u.h + ':' + a;
    if (pending !== key) { pending = key; return; }
    pending = '';
    staffAct(a, u.h, m);
  }
  const label = (a, u, text) => (pending === u.h + ':' + a ? 'Tap again to confirm' : text);
  function giveTo(e, u) {
    e.preventDefault();
    if (grantCredits(u.h, give[u.h])) give[u.h] = '';
  }
</script>

<div class="pagehead"><h2>Users</h2><p>Find a person by handle. Without a search you see the newest accounts.</p></div>
<form id="staffSearch" class="row" autocomplete="off" onsubmit={search}>
  <input id="staffQuery" bind:value={q} maxlength="17" autocapitalize="off" placeholder="@handle" aria-label="Search people">
  <button type="submit" id="staffFind">Search</button>
</form>
<p class="hint" id="staffUsersNote">{chat.adm.q ? 'Results for "' + chat.adm.q + '"' : 'Newest accounts'} ({chat.adm.users.length}).</p>

<div class="ulist" id="staffUsers">
  {#each chat.adm.users as u (u.h)}
    <article class="card pad ucard2" data-handle={u.h}>
      <div class="uhead">
        <Avatar handle={u.h} size={44} />
        <div class="uid">
          <b>@{u.h}</b>
          {#if u.o !== 'user'}<span class={'bd r-' + u.o}>{u.o === 'admin' ? 'ADMIN' : 'MOD'}</span>{:else}<span class="bd">{TIERS[tierOf(u.r)]}</span>{/if}
          <span class={'dot' + (u.on ? ' on' : '')} title={u.on ? 'Online' : 'Offline'}></span>
        </div>
        <span class="chips">
          {#if u.bn}<span class="tag2 fee">Banned</span>{/if}{#if u.mu}<span class="tag2 fee">Muted {u.mu} min</span>{/if}
          {#if u.ki}<span class="tag2 fee">Kicked {u.ki} min</span>{/if}
          {#if !u.g}<span class="tag2">No password</span>{/if}
        </span>
      </div>
      <p class="hint">Joined {joined(u.j)}{u.ls ? ' · seen ' + seen(u.ls) : ''} · {u.p.toLocaleString()} points{u.c != null ? ' · ' + u.c.toLocaleString() + ' credits' : ''}</p>

      {#if reach(u)}
        <div class="uactions2">
          <span class="row">
            <select aria-label={'Minutes for ' + u.h} onchange={e => (mins[u.h] = Number(e.currentTarget.value))}>
              <option value="10" selected={(mins[u.h] || 10) === 10}>10 minutes</option>
              <option value="60" selected={mins[u.h] === 60}>1 hour</option>
              <option value="1440" selected={mins[u.h] === 1440}>24 hours</option>
            </select>
            {#if chat.role === 'admin' && u.o !== 'admin'}
              {#if u.bn}<button type="button" class="ghost small" data-act="unban" onclick={() => unbanUser(u.h)}>Unban</button>
              {:else}<button type="button" class="warn small" data-act="ban" onclick={() => { if (confirm('Ban @' + u.h + ' for good? They cannot sign in until an admin unbans them.')) banUser(u.h, ''); }}>Ban for good</button>{/if}
            {/if}
            <button type="button" class="ghost small" data-act="mute" onclick={() => act('mute', u, mins[u.h] || 10)}>{label('mute', u, 'Mute')}</button>
            <button type="button" class="warn small" data-act="kick" onclick={() => act('kick', u, mins[u.h] || 10)}>{label('kick', u, 'Kick')}</button>
            {#if u.mu}<button type="button" class="ghost small" data-act="unmute" onclick={() => staffAct('unmute', u.h)}>Unmute</button>{/if}
          </span>
          <button type="button" class="ghost small" data-act="clearbio" onclick={() => act('clearbio', u, 0)}>{label('clearbio', u, 'Clear bio')}</button>
        </div>
        {#if area === 'admin' && admin}
          <div class="uactions2">
            {#if u.o === 'user'}
              <button type="button" class="staffbtn mod small" data-act="promote" disabled={!u.g} title={u.g ? '' : 'They need a password first'} onclick={() => act('promote', u, 0)}>{label('promote', u, 'Make moderator')}</button>
              {#if !u.g}<small class="hint">They need a password before they can be made a moderator.</small>{/if}
            {:else}
              <button type="button" class="warn small" data-act="demote" onclick={() => act('demote', u, 0)}>{label('demote', u, 'Remove moderator')}</button>
            {/if}
            <form class="row" onsubmit={e => giveTo(e, u)}>
              <input type="number" inputmode="numeric" min="1" max="100000" placeholder="Credits" aria-label={'Credits for ' + u.h} bind:value={give[u.h]} style="max-width:110px">
              <button type="submit" class="ghost small" data-act="grant">Give</button>
            </form>
          </div>
        {/if}
      {:else}
        <p class="hint">{mine(u) ? 'This is you.' : 'Admin accounts cannot be acted on.'}</p>
      {/if}
    </article>
  {/each}
  {#if chat.adm.users.length === 0}<p class="hint empty">No one matches that search.</p>{/if}
</div>
