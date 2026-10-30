<script>
  import { chat, closeComplaint, banUser } from '../lib/chat.svelte.js';
  const when = ts => new Date(ts).toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
  const WHY = { abuse: 'Abuse of power', harassment: 'Harassment', unfair: 'Unfair action', other: 'Something else' };
  let notes = $state({});
</script>

<div class="pagehead"><h2>Staff complaints</h2><p>Reports about admins and moderators. Only admins see this page. A complaint about you is never shown to you.</p></div>
<div id="complaints" class="ulist">
  {#if chat.complaints.length === 0}<div class="rep meta card pad">No open complaints.</div>{/if}
  {#each chat.complaints as c (c.id)}
    <article class="rep card pad" data-complaint={c.id}>
      <div class="meta">@{c.by} complained about <b>@{c.h}</b> ({c.o === 'admin' ? 'admin' : 'moderator'}) for <span class="tag2 fee">{WHY[c.c] || c.c}</span> · {when(c.ts)}</div>
      {#if c.b}<blockquote class="what">{c.b}</blockquote>{/if}
      {#if c.d}<blockquote class="what">{c.d}</blockquote>{/if}
      <input class="small" bind:value={notes[c.id]} maxlength="300" placeholder="Note for the record (optional)" aria-label="Note">
      <div class="acts">
        <button type="button" class="ghost small" onclick={() => closeComplaint(c.id, 'resolved', notes[c.id])}>Resolved</button>
        <button type="button" class="ghost small" onclick={() => closeComplaint(c.id, 'dismissed', notes[c.id])}>Dismiss</button>
        {#if c.o !== 'admin'}<button type="button" class="warn small" onclick={() => banUser(c.h, 'complaint ' + c.id)}>Ban for good</button>{/if}
      </div>
    </article>
  {/each}
</div>
