<script>
  import { chat, openNote, readAllNotes } from '../lib/chat.svelte.js';
  import Icon from './Icon.svelte';
  import Avatar from './Avatar.svelte';

  const ago = ts => {
    const m = Math.max(0, Math.round((Date.now() - ts) / 60000));
    if (m < 1) return 'now';
    if (m < 60) return m + ' min';
    if (m < 1440) return Math.round(m / 60) + ' h';
    return new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short' });
  };
  const ICON = { dm: 'chat', tag: 'at', reply: 'at', friend: 'users', tip: 'gift' };
  function text(n) {
    const times = n.n > 1 ? ' (' + n.n + ')' : '';
    switch (n.k) {
      case 'dm': return n.n > 1 ? '@' + n.a + ' sent you ' + n.n + ' messages' : '@' + n.a + ' sent you a message';
      case 'tag': return '@' + n.a + ' tagged you in ' + (n.rn || 'a room') + times;
      case 'reply': return '@' + n.a + ' replied to you in ' + (n.rn || 'a room') + times;
      case 'friend': return n.b === 'accepted' ? '@' + n.a + ' accepted your friend request' : '@' + n.a + ' sent you a friend request';
      case 'tip': return '@' + n.a + ' sent you a ' + n.b + ' (' + n.m + ' credit' + (n.m === 1 ? '' : 's') + ')' + (n.rn ? ' in ' + n.rn : '');
    }
    return '@' + n.a;
  }
  const detail = n => (n.k === 'dm' || n.k === 'tag' || n.k === 'reply' ? n.b : '');
</script>

<section id="notesPage" hidden={chat.view !== 'notes'}>
  <div class="pagehead row between">
    <div>
      <h2>Notifications</h2>
      <p>What happened while you were away.</p>
    </div>
    <button type="button" id="notesAll" class="ghost small" disabled={chat.notes.unread === 0} onclick={readAllNotes}>Mark all read</button>
  </div>

  <ul id="noteList" class="card ledger notes">
    {#if !chat.notes.loaded}
      <li class="empty" id="notesLoading">Loading.</li>
    {:else if chat.notes.list.length === 0}
      <li class="empty" id="notesEmpty">Nothing yet. Messages, tags, replies, gifts and friend requests show up here.</li>
    {/if}
    {#each chat.notes.list as n (n.i)}
      <li class={n.u ? 'unread' : ''} data-id={n.i}>
        <button type="button" class="noterow" onclick={() => openNote(n)}>
          <span class="nicon"><Avatar handle={n.a} size={38} /><i class={'kind-badge k-' + n.k}><Icon name={ICON[n.k] || 'bell'} size={12} /></i></span>
          <span class="nbody"><b>{text(n)}</b>{#if detail(n)}<small>{detail(n)}</small>{/if}</span>
          <time>{ago(n.ts)}</time>
          {#if n.u}<span class="udot" aria-label="Unread"></span>{/if}
        </button>
      </li>
    {/each}
  </ul>
</section>
