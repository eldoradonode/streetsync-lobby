<script>
  import { chat, sendDM, friendAct, blockAct, reportDM } from '../lib/chat.svelte.js';
  import { tick } from 'svelte';

  let box, textEl;
  let stick = true;
  let reporting = $state(0); // id of the message whose report choices are open

  const t = $derived(chat.thread);
  const mine = m => !!chat.me && m.f.toLowerCase() === chat.me.toLowerCase();
  const time = ts => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });

  function onScroll() {
    stick = box.scrollHeight - box.scrollTop - box.clientHeight < 80;
  }

  $effect(() => {
    t.items.length;
    chat.view;
    tick().then(() => { if (stick && box) box.scrollTop = box.scrollHeight; });
  });

  function submit(e) {
    e.preventDefault();
    if (sendDM(textEl.value)) {
      textEl.value = '';
      stick = true;
    }
    textEl.focus();
  }

  const report = (id, why) => { if (reportDM(id, why)) reporting = 0; };
</script>

<section id="thread" hidden={chat.view !== 'thread'}>
  {#if t.s !== 'friend'}
    <div id="tbar">
      <div class="tstate">
        {#if t.s === 'sent'}
          <span class="st">Friend request sent</span>
          <button type="button" class="ghost small" onclick={() => friendAct('cancel', t.h)}>Cancel</button>
        {:else if t.s === 'got'}
          <span class="st">@{t.h} wants to be friends</span>
          <button type="button" class="small" onclick={() => friendAct('accept', t.h)}>Accept</button>
          <button type="button" class="ghost small" onclick={() => friendAct('decline', t.h)}>Decline</button>
        {:else if t.s === 'blocked'}
          <span class="st">You blocked @{t.h}</span>
          <button type="button" class="ghost small" onclick={() => blockAct('del', t.h)}>Unblock</button>
        {:else}
          <span class="st">Not friends yet</span>
          <button type="button" class="small" onclick={() => friendAct('add', t.h)}>Add friend</button>
        {/if}
      </div>
      {#if t.s !== 'blocked'}
        <p id="costNote" class="hint">
          {#if t.cost > 0}
            Each message you send costs {t.cost} credit{t.cost === 1 ? '' : 's'} until you are friends. You have {chat.credits.toLocaleString()}.
          {:else}
            Replying is free for you.
          {/if}
        </p>
      {/if}
    </div>
  {/if}

  <div id="tlog" role="log" aria-live="polite" bind:this={box} onscroll={onScroll}>
    {#if t.ready && t.items.length === 0}
      <div class="m sys">Say hello to @{t.h}.</div>
    {/if}
    {#each t.items as m (m.i)}
      <div class={'dm' + (mine(m) ? ' mine' : '')} data-id={m.i}>
        <div class="bubble">{m.b}</div>
        <div class="meta">
          <time>{time(m.ts)}</time>
          {#if !mine(m)}
            <button type="button" class="link" onclick={() => (reporting = reporting === m.i ? 0 : m.i)}>Report</button>
          {/if}
        </div>
        {#if reporting === m.i}
          <div class="actions">
            <span class="hint">Why?</span>
            <button type="button" class="ghost" onclick={() => report(m.i, 'spam')}>Spam</button>
            <button type="button" class="ghost" onclick={() => report(m.i, 'abuse')}>Abuse</button>
            <button type="button" class="ghost" onclick={() => report(m.i, 'other')}>Other</button>
          </div>
        {/if}
      </div>
    {/each}
  </div>

  <form id="tsend" class="row" autocomplete="off" onsubmit={submit} hidden={t.s === 'blocked'}>
    <input id="ttext" bind:this={textEl} maxlength="300" placeholder="Write a message" aria-label="Message">
    <button type="submit">Send</button>
  </form>
</section>
