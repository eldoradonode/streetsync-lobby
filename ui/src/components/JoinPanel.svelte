<script>
  import { chat, confirmJoin, cancelJoin } from '../lib/chat.svelte.js';

  let pw;
  const p = $derived(chat.joinPanel);
</script>

{#if p}
  <form id="rpPanel" class="pcard" autocomplete="off" onsubmit={e => { e.preventDefault(); confirmJoin(pw ? pw.value : ''); }}>
    <div class="jtitle"><b>{p.n}</b>{#if p.o}<small> hosted by @{p.o}</small>{/if}</div>
    {#if p.f > 0}
      <p class="hint" id="rpFee">Getting in costs {p.f} credit{p.f === 1 ? '' : 's'}, once. You have {chat.credits.toLocaleString()}.</p>
    {/if}
    {#if p.w}
      <input id="rpPw" type="password" bind:this={pw} placeholder="Room password" maxlength="32" autocomplete="off" aria-label="Room password">
    {/if}
    {#if p.err}<p class="err" id="rpErr">{p.err}</p>{/if}
    <div class="row">
      <button type="submit" id="rpGo">{p.f > 0 ? 'Pay ' + p.f + ' and enter' : 'Enter'}</button>
      <button type="button" class="ghost" id="rpNo" onclick={cancelJoin}>Cancel</button>
    </div>
  </form>
{/if}
