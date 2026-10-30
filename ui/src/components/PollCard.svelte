<script>
  import { chat, canManageRoom, createPoll, votePoll, closePoll } from '../lib/chat.svelte.js';
  import { percents, POLL_LIMITS } from '../lib/engage.js';

  let fold = $state(false);
  let q = $state('');
  let opts = $state(['', '']);
  let err = $state('');
  let confirmClose = $state(false);

  const p = $derived(chat.poll);
  const pct = $derived(p ? percents(p.n) : []);
  const manage = $derived(canManageRoom());
  const canStart = $derived(manage && (!p || p.c));

  // The form keeps what was typed until the server confirms the poll, so a failure can be fixed and sent again.
  function start(e) {
    e.preventDefault();
    if (chat.pollBusy) return;
    err = createPoll(q, opts);
  }
  $effect(() => { if (!chat.pollForm) { q = ''; opts = ['', '']; err = ''; } });
  function cancel() { chat.pollForm = false; err = ''; chat.pollErr = ''; }
  function closeIt() {
    if (!confirmClose) { confirmClose = true; setTimeout(() => (confirmClose = false), 4000); return; }
    confirmClose = false;
    closePoll();
  }
</script>

{#if p || canStart}
  <div id="pollbox">
    {#if p}
      <div class={'pollcard' + (p.c ? ' closed' : '')}>
        <button type="button" class="pollhead" aria-expanded={!fold} onclick={() => (fold = !fold)}>
          <span class="pollicon" aria-hidden="true">&#128202;</span>
          <span class="pollq">{p.q}</span>
          <span class="pollstate">{p.c ? 'Closed' : p.y >= 0 ? 'Voted' : 'Vote'}</span>
        </button>
        {#if !fold}
          <div class="pollopts" role="group" aria-label="Poll options">
            {#each p.o as label, i (i)}
              <button type="button" class={'popt' + (p.y === i ? ' mine' : '')} disabled={p.c} aria-pressed={p.y === i} onclick={() => votePoll(i)}>
                <span class="pbar" style:width={pct[i] + '%'}></span>
                <span class="plabel">{#if p.y === i}&#10003; {/if}{label}</span>
                <span class="pnum">{pct[i]}% <small>({p.n[i]})</small></span>
              </button>
            {/each}
          </div>
          <div class="pollfoot">
            <small>{p.t} vote{p.t === 1 ? '' : 's'} &middot; by @{p.by}{p.c ? ' &middot; closed' : p.y >= 0 ? ' &middot; tap another option to change your vote' : ''}</small>
            {#if manage && !p.c}
              <button type="button" class={confirmClose ? 'warn' : 'ghost'} onclick={closeIt}>{confirmClose ? 'Tap again to close' : 'Close poll'}</button>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
    {#if canStart && !chat.pollForm}
      <button type="button" class="ghost pollnew" onclick={() => (chat.pollForm = true)}>&#65291; Start a poll</button>
    {/if}
    {#if canStart && chat.pollForm}
      <form class="pollform" onsubmit={start} autocomplete="off">
        <input maxlength={POLL_LIMITS.qMax} placeholder="Ask a question" aria-label="Poll question" bind:value={q}>
        {#each opts as _, i (i)}
          <div class="polloptrow">
            <input maxlength={POLL_LIMITS.optLen} placeholder={'Option ' + (i + 1)} aria-label={'Option ' + (i + 1)} bind:value={opts[i]}>
            {#if opts.length > POLL_LIMITS.optMin}<button type="button" class="ghost" aria-label={'Remove option ' + (i + 1)} onclick={() => opts.splice(i, 1)}>&#10005;</button>{/if}
          </div>
        {/each}
        {#if opts.length < POLL_LIMITS.optMax}<button type="button" class="ghost" onclick={() => opts.push('')}>Add option</button>{/if}
        {#if err || chat.pollErr}<div class="pollerr" role="alert">{chat.pollErr || err}</div>{/if}
        <div class="pollbtns"><button type="submit" disabled={chat.pollBusy} aria-busy={chat.pollBusy}>{chat.pollBusy ? 'Starting\u2026' : 'Start poll'}</button><button type="button" class="ghost" onclick={cancel}>Cancel</button></div>
      </form>
    {/if}
  </div>
{/if}
