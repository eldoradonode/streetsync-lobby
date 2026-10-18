<script>
  import Avatar from './Avatar.svelte';
  import { chat, openThread, openUser, friendAct, blockAct, isSplit } from '../lib/chat.svelte.js';

  let who;
  let tab = $state('chats'); // chats, friends, requests or blocked
  let menu = $state(''); // the friend whose actions are open

  const when = ts => {
    const d = new Date(ts), now = new Date();
    return d.toDateString() === now.toDateString()
      ? d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      : d.toLocaleDateString([], { day: 'numeric', month: 'short' });
  };
  let q = $state('');
  const split = $derived(isSplit());
  const shown = $derived(q.trim() ? chat.chats.filter(c => (c.h + ' ' + c.b).toLowerCase().includes(q.trim().toLowerCase())) : chat.chats);
  const open = c => chat.view === 'thread' && c.h.toLowerCase() === chat.thread.h.toLowerCase();
  const unreadChats = $derived(chat.chats.reduce((n, c) => n + (c.u || 0), 0));

  function find(e) {
    e.preventDefault();
    if (openThread(who.value)) who.value = '';
  }
</script>

<div id="inbox" class={split ? 'split' : ''} hidden={chat.view !== 'inbox' && !split}>
  <form id="dmFind" class="row" autocomplete="off" onsubmit={find}>
    <input id="dmWho" bind:this={who} placeholder="@handle to message" maxlength="17" autocapitalize="none" aria-label="Handle to message">
    <button type="submit">Open</button>
  </form>

  <div id="inboxTabs" role="tablist" aria-label="Messages">
    <button type="button" role="tab" id="tabChats" aria-selected={tab === 'chats'} class={tab === 'chats' ? 'active' : ''} onclick={() => (tab = 'chats')}>
      Chats{#if unreadChats > 0}<span class="badge">{unreadChats}</span>{/if}
    </button>
    <button type="button" role="tab" id="tabFriends" aria-selected={tab === 'friends'} class={tab === 'friends' ? 'active' : ''} onclick={() => (tab = 'friends')}>
      Friends{#if chat.friends.length > 0}<span class="count">{chat.friends.length}</span>{/if}
    </button>
    <button type="button" role="tab" id="tabRequests" aria-selected={tab === 'requests'} class={tab === 'requests' ? 'active' : ''} onclick={() => (tab = 'requests')}>
      Requests{#if chat.incoming.length > 0}<span class="badge">{chat.incoming.length}</span>{/if}
    </button>
    <button type="button" role="tab" id="tabBlocked" aria-selected={tab === 'blocked'} class={tab === 'blocked' ? 'active' : ''} onclick={() => (tab = 'blocked')}>
      Blocked{#if chat.blockedUsers.length > 0}<span class="count">{chat.blockedUsers.length}</span>{/if}
    </button>
  </div>

  {#if tab === 'chats'}
    {#if chat.chats.length > 3}
      <input id="chatSearch" type="search" bind:value={q} placeholder="Search conversations" aria-label="Search conversations" maxlength="40" autocomplete="off">
    {/if}
    <ul id="chatList" class="plist">
      {#if chat.chats.length === 0}
        <li class="empty">No messages yet. Tap a name in a room and choose Message.</li>
      {:else if shown.length === 0}
        <li class="empty" id="chatNoMatch">No conversation matches "{q.trim()}".</li>
      {/if}
      {#each shown as c (c.h)}
        <li class={open(c) ? 'sel' : ''}>
          <button type="button" class="chatrow" aria-current={open(c) ? 'true' : undefined} onclick={() => openThread(c.h)}>
            <Avatar handle={c.h} size={38} />
            <span class="cbody"><b>@{c.h}</b><small>{c.m ? 'You: ' : ''}{c.b}</small></span>
            <time>{when(c.ts)}</time>
            {#if c.u > 0}<span class="badge">{c.u}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  {:else if tab === 'friends'}
    <ul id="friendList" class="plist">
      {#if chat.friends.length === 0}
        <li class="empty">No friends yet. Open someone's profile and tap Add friend.</li>
      {/if}
      {#each chat.friends as h (h)}
        <li class="frow">
          <button type="button" class="chatrow" aria-label={'Open the profile of ' + h} onclick={() => openUser(h)}>
            <Avatar handle={h} size={38} />
            <span class="cbody"><b>@{h}</b><small>Messages are free</small></span>
          </button>
          <button type="button" class="small" onclick={() => openThread(h)}>Message</button>
          <button type="button" class="ghost small more" aria-label={'More for ' + h} aria-expanded={menu === h} onclick={() => (menu = menu === h ? '' : h)}>&#8943;</button>
          {#if menu === h}
            <div class="actions fa">
              <button type="button" class="ghost" onclick={() => { menu = ''; friendAct('remove', h); }}>Remove friend</button>
              <button type="button" class="warn" onclick={() => { menu = ''; blockAct('add', h); }}>Block</button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {:else if tab === 'requests'}
    <ul id="reqList" class="plist">
      {#if chat.incoming.length === 0}
        <li class="empty">No friend requests waiting.</li>
      {/if}
      {#each chat.incoming as h (h)}
        <li>
          <button type="button" class="chatrow" aria-label={'Open the profile of ' + h} onclick={() => openUser(h)}>
            <Avatar handle={h} size={34} />
            <span class="cbody"><b>@{h}</b><small>wants to be friends</small></span>
          </button>
          <button type="button" class="small" onclick={() => friendAct('accept', h)}>Accept</button>
          <button type="button" class="ghost small" onclick={() => friendAct('decline', h)}>Decline</button>
        </li>
      {/each}
    </ul>
    {#if chat.outgoing.length}
      <h2>Waiting for an answer</h2>
      <ul id="sentList" class="plist">
        {#each chat.outgoing as h (h)}
          <li>
            <button type="button" class="chatrow" aria-label={'Open the profile of ' + h} onclick={() => openUser(h)}>
              <Avatar handle={h} size={34} />
              <span class="cbody"><b>@{h}</b></span>
            </button>
            <button type="button" class="ghost small" onclick={() => friendAct('cancel', h)}>Cancel</button>
          </li>
        {/each}
      </ul>
    {/if}
  {:else}
    <ul id="blockList" class="plist">
      {#if chat.blockedUsers.length === 0}
        <li class="empty">You have not blocked anyone.</li>
      {/if}
      {#each chat.blockedUsers as h (h)}
        <li>
          <button type="button" class="chatrow" aria-label={'Open the profile of ' + h} onclick={() => openUser(h)}>
            <Avatar handle={h} size={34} />
            <span class="cbody"><b>@{h}</b></span>
          </button>
          <button type="button" class="ghost small" onclick={() => blockAct('del', h)}>Unblock</button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
