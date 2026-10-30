<script>
  import { chat, canManageRoom, unpinMessage } from '../lib/chat.svelte.js';
  import { pinSnippet } from '../lib/engage.js';

  let open = $state(false);

  // Tap: go to the message when it is on screen, otherwise show the whole pinned text right here.
  function go() {
    const el = document.querySelector('.m[data-id="' + ((chat.pin && chat.pin.i) | 0) + '"]');
    if (el) {
      open = false;
      el.scrollIntoView({ block: 'center', behavior: 'smooth' });
      el.classList.add('flash');
      setTimeout(() => el.classList.remove('flash'), 1500);
    } else open = !open;
  }
</script>

{#if chat.pin}
  <div id="pinbar" role="note" aria-label="Pinned message">
    <div class="pinrow">
      <button type="button" class="pinmain" aria-expanded={open} onclick={go}>
        <span class="pinicon" aria-hidden="true">&#128204;</span>
        <span class="pintext"><b>@{chat.pin.h}</b> {open ? '' : pinSnippet(chat.pin.b)}</span>
      </button>
      {#if canManageRoom()}
        <button type="button" class="ghost pinx" aria-label="Unpin this message" onclick={unpinMessage}>&#10005;</button>
      {/if}
    </div>
    {#if open}<div class="pinfull">{chat.pin.b}</div>{/if}
  </div>
{/if}
