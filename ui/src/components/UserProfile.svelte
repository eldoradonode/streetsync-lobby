<script>
  import Avatar from './Avatar.svelte';
  import { chat, openThread, friendAct, blockAct, go, openReport, isMod } from '../lib/chat.svelte.js';
  import { TIERS, tierOf } from '../lib/rank.js';

  const u = $derived(chat.userView);
  const self = $derived(!!chat.me && u.h.toLowerCase() === chat.me.toLowerCase());
  const tier = $derived(u.r != null ? tierOf(u.r) : -1);
  const joined = $derived(u.j ? new Date(u.j).toLocaleDateString([], { month: 'long', year: 'numeric' }) : '');
</script>

<section id="userProfile" hidden={chat.view !== 'user'}>
  <div class="ucard pcard">
    <Avatar handle={u.h || 'x'} size={96} ring="var(--accent)" />
    <div class="uname">
      <h2 id="userName">@{u.h}</h2>
      {#if u.o}<span class={'bd r-' + u.o}>{u.o === 'admin' ? 'ADMIN' : 'MOD'}</span>{/if}
    </div>
    {#if u.ready}
      <p class="ustate" id="userState">
        <span class={'dot' + (u.on ? ' on' : '')} aria-hidden="true"></span>{u.on ? 'Online now' : 'Offline'}
      </p>
      {#if u.bio}<p class="bioline" id="userBio">{u.bio}</p>{/if}
      <dl class="ufacts">
        {#if tier >= 0}
          <div><dt>Tier</dt><dd id="userTier">{TIERS[tier]}</dd></div>
          <div><dt>Points</dt><dd id="userPoints">{(u.p || 0).toLocaleString()}</dd></div>
        {/if}
        {#if joined}<div><dt>Member since</dt><dd id="userJoined">{joined}</dd></div>{/if}
      </dl>
      <div class="uactions">
        {#if self}
          <p class="hint">This is you.</p>
          <button type="button" id="userMine" onclick={() => go('profile')}>My wallet and settings</button>
        {:else}
          {#if !u.sh || u.dmok}<button type="button" id="userMessage" onclick={() => openThread(u.h)}>Message</button>{/if}
          {#if u.s === 'none' && !u.sh}
            <button type="button" class="ghost" id="userFriend" onclick={() => friendAct('add', u.h)}>Add friend</button>
          {:else if u.s === 'sent'}
            <button type="button" class="ghost" id="userCancel" onclick={() => friendAct('cancel', u.h)}>Cancel request</button>
          {:else if u.s === 'got'}
            <button type="button" class="ghost" id="userAccept" onclick={() => friendAct('accept', u.h)}>Accept request</button>
            <button type="button" class="ghost" id="userDecline" onclick={() => friendAct('decline', u.h)}>Decline</button>
          {:else if u.s === 'friend'}
            <button type="button" class="ghost" id="userRemove" onclick={() => friendAct('remove', u.h)}>Remove friend</button>
          {/if}
          {#if u.s === 'blocked'}
            <button type="button" class="ghost" id="userUnblock" onclick={() => blockAct('del', u.h)}>Unblock</button>
          {:else}
            <button type="button" class="warn" id="userBlock" onclick={() => blockAct('add', u.h)}>Block</button>
          {/if}
          {#if u.o}
            <button type="button" class="ghost" id="userReportStaff" onclick={() => openReport({ kind: 'staff', h: u.h })}>Report staff misconduct</button>
          {:else}
            <button type="button" class="ghost" id="userReport" onclick={() => openReport({ kind: 'user', h: u.h })}>Report</button>
          {/if}
          {#if u.sh}<p class="hint">Staff cannot be tipped or sent friend requests. If one of them wrote to you, you can reply.</p>{/if}
        {/if}
      </div>
    {:else}
      <p class="hint">Loading</p>
    {/if}
  </div>
</section>
