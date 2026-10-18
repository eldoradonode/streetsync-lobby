<script>
  import Avatar from './Avatar.svelte';
  import { chat, staffAct, flash } from '../lib/chat.svelte.js';

  let who = $state('');
  let pending = $state('');
  const fmt = ts => new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short', year: 'numeric' });
  const mods = $derived(chat.adm.staff.filter(s => s.o === 'mod'));
  const admins = $derived(chat.adm.staff.filter(s => s.o === 'admin'));

  function add(e) {
    e.preventDefault();
    const h = who.trim().replace(/^@/, '');
    if (!/^[A-Za-z0-9_]{3,16}$/.test(h)) { flash('Enter a full handle, 3 to 16 letters, numbers or underscores.'); return; }
    staffAct('promote', h);
    who = '';
  }
  function revoke(s) {
    if (pending !== s.h) { pending = s.h; return; }
    pending = '';
    staffAct('demote', s.h);
  }
</script>

<div class="pagehead"><h2>Moderators</h2><p>Who can moderate, and how to add or remove them.</p></div>

<form id="addMod" class="card pad" autocomplete="off" onsubmit={add}>
  <label for="addModWho">Make someone a moderator</label>
  <div class="row">
    <input id="addModWho" bind:value={who} maxlength="17" autocapitalize="off" placeholder="@handle">
    <button type="submit" id="addModGo" class="staffbtn mod">Make moderator</button>
  </div>
  <p class="hint">They need an account with a password. Find them under Users if you are not sure of the handle.</p>
</form>

<div class="sechead"><h3>Moderators ({mods.length})</h3></div>
<div class="ulist" id="modList">
  {#each mods as s (s.h)}
    <article class="card pad ucard2" data-handle={s.h}>
      <div class="uhead">
        <Avatar handle={s.h} size={44} />
        <div class="uid"><b>@{s.h}</b><span class="bd r-mod">MOD</span><span class={'dot' + (s.on ? ' on' : '')}></span></div>
        <button type="button" class="warn small" data-act="demote" onclick={() => revoke(s)}>{pending === s.h ? 'Tap again to confirm' : 'Remove moderator'}</button>
      </div>
      <p class="hint">{s.by ? 'Made moderator by @' + s.by + (s.at ? ' on ' + fmt(s.at) : '') : 'Made moderator before records began'} · last seen {fmt(s.ls)}</p>
    </article>
  {/each}
  {#if mods.length === 0}<p class="hint empty" id="noMods">There are no moderators yet.</p>{/if}
</div>

<div class="sechead"><h3>Admins ({admins.length})</h3></div>
<div class="ulist" id="adminList">
  {#each admins as s (s.h)}
    <article class="card pad ucard2" data-handle={s.h}>
      <div class="uhead"><Avatar handle={s.h} size={44} /><div class="uid"><b>@{s.h}</b><span class="bd r-admin">ADMIN</span><span class={'dot' + (s.on ? ' on' : '')}></span></div></div>
      <p class="hint">Admin accounts are set on the server and cannot be changed here.</p>
    </article>
  {/each}
</div>
