<script>
  import { chat, sendDM, friendAct, blockAct, openDMReport, acceptPeerKey, verifyPeer, go } from '../lib/chat.svelte.js';
  import { tick } from 'svelte';

  let box, textEl;
  let stick = true;
  let showCode = $state(false);

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

  const sec = $derived(t.sec || { state: 'loading' });
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

  <div id="esec" class={'esec ' + sec.state}>
    {#if sec.state === 'changed'}
      <p class="warn-text"><b>@{t.h}'s encryption key changed.</b> This can happen when they use a new device, but it is also what an impersonator looks like. Compare the safety code with them in person or on a call before you accept it. You cannot send until you do.</p>
      {#if sec.code}<p class="safety" id="safetyCode">{sec.code}</p>{/if}
      <div class="row"><button type="button" id="acceptKey" class="warn small" onclick={acceptPeerKey}>Accept the new key</button></div>
    {:else if sec.state === 'peer_none'}
      <p class="hint">@{t.h} has not set up encrypted messages yet, so you cannot message them. They need to open STREETSYNC once on the device they use.</p>
    {:else if sec.state === 'unsupported'}
      <p class="hint warn-text">This browser cannot encrypt messages, so direct messages are switched off here.</p>
    {:else if chat.e2e.state === 'mismatch' || chat.e2e.state === 'other_device'}
      <p class="hint warn-text">Your encryption key is not on this device, so you can not read or send new messages here. <button type="button" class="link" onclick={() => go('settings')}>Open Settings</button></p>
    {:else if sec.state === 'ok'}
      <p class="hint" id="encNote">
        <span aria-hidden="true">&#128274;</span> End-to-end encrypted. Only you and @{t.h} can read new messages.
        {#if sec.verified}<b id="verifiedTag">Verified</b>{:else}<button type="button" id="showCode" class="link" onclick={() => (showCode = !showCode)}>{showCode ? 'Hide safety code' : 'Verify'}</button>{/if}
      </p>
      {#if showCode && !sec.verified}
        <div class="safetybox" id="safetyBox">
          <p class="hint">Compare this code with @{t.h} in person or on a call. If it matches on both phones, nobody swapped your keys.</p>
          <p class="safety" id="safetyCode">{sec.code}</p>
          <button type="button" id="markVerified" class="small" onclick={() => { verifyPeer(); showCode = false; }}>It matches</button>
        </div>
      {/if}
    {/if}
  </div>

  <div id="tlog" role="log" aria-live="polite" bind:this={box} onscroll={onScroll}>
    {#if t.ready && t.items.length === 0}
      <div class="m sys">Say hello to @{t.h}.</div>
    {/if}
    {#each t.items as m (m.i)}
      <div class={'dm' + (mine(m) ? ' mine' : '')} data-id={m.i}>
        {#if m.legacy}
          <div class="bubble legacy">{m.b}</div>
          <div class="meta"><small class="hint">Not encrypted (sent before encrypted messages). It is deleted with the 7 day cleanup.</small></div>
        {:else if m.err}
          <div class="bubble undecryptable"><i>{m.err === 'no_private' ? 'Cannot be read on this device. It was made for a key held on another device.' : m.err === 'no_peer_key' ? 'Not shown. It uses a key you have not accepted.' : 'Cannot be read. It failed its integrity check.'}</i></div>
        {:else}
          <div class="bubble">{m.b}</div>
        {/if}
        <div class="meta">
          <time>{time(m.ts)}</time>
          {#if mine(m)}<span class={'ticks t' + (m.st || 1)} data-st={m.st || 1} role="img" aria-label={['Sent', 'Delivered', 'Read'][(m.st || 1) - 1]}>{(m.st || 1) > 1 ? '\u2713\u2713' : '\u2713'}</span>{/if}
          {#if !mine(m) && (m.legacy || !m.err)}
            <button type="button" class="link" id={'rep' + m.i} onclick={() => openDMReport(m.i)}>Report</button>
          {/if}
        </div>
      </div>
    {/each}
  </div>

  {#if t.closed && t.s !== 'blocked'}
    <p id="closedNote" class="hint closednote">@{t.h} only takes messages from friends. Add them as a friend first.</p>
  {/if}
  <form id="tsend" class="row" autocomplete="off" onsubmit={submit} hidden={t.s === 'blocked'}>
    <input id="ttext" bind:this={textEl} maxlength="300" placeholder={t.closed ? 'Friends only' : 'Write a message'} aria-label="Message" disabled={t.closed}>
    <button type="submit" disabled={t.closed || sec.state !== 'ok' || chat.e2e.state !== 'ready'}>Send</button>
  </form>
</section>
