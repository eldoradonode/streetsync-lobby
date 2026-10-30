<script>
  import { chat } from '../lib/chat.svelte.js';
  const WORDS = { mute: 'muted', unmute: 'unmuted', kick: 'removed', del: 'deleted a message from', promote: 'made a moderator', demote: 'removed as moderator', dismiss: 'dismissed', grant: 'gave credits to', close_room: 'closed a room of', clearbio: 'cleared the bio of' };
  const when = ts => new Date(ts).toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
</script>

<div class="pagehead"><h2>Audit log</h2><p>The latest {chat.adm.log.length} staff actions, newest first.</p></div>
<ul id="auditLog" class="card ledger">
  {#if chat.adm.log.length === 0}<li class="empty">Nothing has been logged yet.</li>{/if}
  {#each chat.adm.log as l, i (i)}
    <li>
      <span class="wnote"><span><b>@{l.a}</b> {WORDS[l.x] || l.x}{l.h ? ' @' + l.h : ''}</span>{#if l.d}<small>{l.d}</small>{/if}</span>
      <small>{when(l.ts)}</small>
    </li>
  {/each}
</ul>
