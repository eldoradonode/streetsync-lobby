<script>
  import { chat, modReport, banUser } from '../lib/chat.svelte.js';
  const when = ts => new Date(ts).toLocaleString([], { day: 'numeric', month: 'short', hour: '2-digit', minute: '2-digit' });
</script>

<div class="pagehead"><h2>Reports</h2><p>Messages people flagged. Settle each one: act, or dismiss it.</p></div>
<div id="reports" class="ulist">
  <div id="repList" class="ulist">
    {#if chat.reports.length === 0}
      <div class="rep meta card pad">No open reports.</div>
    {/if}
    {#each chat.reports as r (r.id)}
      <article class="rep card pad">
        <div class="meta">@{r.by} reported {r.k === 'room' ? 'the room ' + r.rm + ' (host @' + r.h + ')' : '@' + r.h} for <span class="tag2 fee">{r.c}</span>{r.k === 'user' ? ' (person)' : ''} · {when(r.ts)}</div>
        {#if r.b}<blockquote class="what">{r.b}</blockquote>{/if}
        {#if r.d}<blockquote class="what">{r.d}</blockquote>{/if}
        <div class="acts">
          <button type="button" class="ghost small" onclick={() => modReport({ t: 'mod', a: 'mute', h: r.h, m: 60, rid: r.id })}>Mute 1 hour</button>
          <button type="button" class="warn small" onclick={() => modReport({ t: 'mod', a: 'kick', h: r.h, m: 10, rid: r.id })}>Kick 10 min</button>
          {#if !r.k}<button type="button" class="warn small" onclick={() => modReport({ t: 'mod', a: 'del', i: r.i, rid: r.id })}>Delete message</button>{/if}
          {#if r.k === 'room'}<button type="button" class="warn small" onclick={() => modReport({ t: 'rclose', r: r.rm, rid: r.id })}>Close room</button>{/if}
          {#if chat.role === 'admin'}<button type="button" class="warn small" onclick={() => banUser(r.h, 'report ' + r.id)}>Ban for good</button>{/if}
          <button type="button" class="ghost small" onclick={() => modReport({ t: 'mod', a: 'dismiss', id: r.id })}>Dismiss</button>
        </div>
      </article>
    {/each}
  </div>
</div>
