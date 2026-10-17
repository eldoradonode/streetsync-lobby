<script>
  import Message from './Message.svelte';
  import HostBar from './HostBar.svelte';
  import TipSheet from './TipSheet.svelte';
  import { chat, sendMessage, clearReply, openUser } from '../lib/chat.svelte.js';
  import { colorFor } from '../lib/avatar.js';
  import { tick } from 'svelte';

  let box, textEl;
  let stick = true;

  function onScroll() {
    stick = box.scrollHeight - box.scrollTop - box.clientHeight < 80;
  }

  // Keep the newest message in view, unless the person has scrolled up to read.
  $effect(() => {
    chat.items.length;
    chat.view;
    tick().then(() => { if (stick && box) box.scrollTop = box.scrollHeight; });
  });

  function submit(e) {
    e.preventDefault();
    if (sendMessage(textEl.value)) {
      textEl.value = '';
      stick = true;
    }
    textEl.focus();
  }
</script>

<section id="chat" hidden={chat.view !== 'room'}>
  <HostBar />
  <TipSheet />
  {#if chat.roomKind && chat.roomOwner && !chat.isHost}
    <div id="roomMeta">Hosted by <button type="button" class="link" onclick={() => openUser(chat.roomOwner)}>@{chat.roomOwner}</button></div>
  {/if}
  <div id="log" role="log" aria-live="polite" bind:this={box} onscroll={onScroll}>
    {#each chat.items as it (it.key)}
      {#if it.kind === 'msg'}
        <Message m={it.m} tagged={it.tagged} />
      {:else}
        <div class={'m ' + it.cls}>{it.text}</div>
      {/if}
    {/each}
  </div>
  <div id="replybar" hidden={!chat.replyTo} style:--qc={chat.replyTo ? colorFor(chat.replyTo.h) : null}>
    <div class="rtext">
      <b id="replyWho">{chat.replyTo ? 'Replying to @' + chat.replyTo.h : ''}</b>
      <span id="replySnip">{chat.replyTo ? (Array.from(chat.replyTo.b).length <= 80 ? chat.replyTo.b : Array.from(chat.replyTo.b).slice(0, 80).join('') + '\u2026') : ''}</span>
    </div>
    <button type="button" id="replyCancel" class="ghost" aria-label="Cancel reply" onclick={() => { clearReply(); textEl.focus(); }}>&#10005;</button>
  </div>
  <form id="send" class="row" autocomplete="off" onsubmit={submit}>
    <input id="text" bind:this={textEl} maxlength="300" placeholder="Say something" aria-label="Message" onkeydown={e => { if (e.key === 'Escape') clearReply(); }}>
    <button type="submit">Send</button>
  </form>
</section>
