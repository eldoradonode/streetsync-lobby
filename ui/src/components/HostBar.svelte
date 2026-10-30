<script>
  import { chat, isMod, saveRoomSettings, closeRoom, inviteLink, flash } from '../lib/chat.svelte.js';

  let feeEl, pwEl, kindEl;
  let confirmClose = $state(false);

  const show = $derived(!!chat.roomKind && (chat.isHost || isMod()));

  async function copy() {
    const link = inviteLink();
    try { await navigator.clipboard.writeText(link); flash('Invite link copied.'); }
    catch { flash(link); }
  }

  function saveFee(e) {
    e.preventDefault();
    const n = Math.floor(Number(feeEl.value));
    if (!(n >= 0 && n <= 50)) { flash('The entry fee is 0 to 50 credits.'); return; }
    saveRoomSettings({ fee: n });
  }
  function savePw(e) {
    e.preventDefault();
    const v = pwEl.value;
    if (v.length < 4 || v.length > 32) { flash('Use a password of 4 to 32 characters.'); return; }
    if (saveRoomSettings({ pw: v })) pwEl.value = '';
  }
  const removePw = () => saveRoomSettings({ pw: '' });
  const saveKind = () => saveRoomSettings({ kind: kindEl.value });
  function close() {
    if (!confirmClose) { confirmClose = true; return; }
    confirmClose = false;
    closeRoom();
  }
</script>

{#if show}
  <details id="hostTools">
    <summary>{chat.isHost ? 'Room tools' : 'Room tools (staff)'}</summary>
    <div class="htools">
      {#if chat.invite}
        <div class="hfield">
          <span class="flabel">Invite link</span>
          <div class="row" id="inviteRow">
            <input id="inviteCode" readonly value={chat.invite} aria-label="Invite code">
            <button type="button" id="copyInvite" onclick={copy}>Copy link</button>
          </div>
        </div>
      {/if}
      <div class="hfield">
        <span class="flabel">Who can find it</span>
        <select id="kindInput" bind:this={kindEl} value={chat.roomKind} onchange={saveKind} aria-label="Who can find the room">
          <option value="public">Public</option>
          <option value="private">Private</option>
        </select>
      </div>
      <div class="hfield">
        <span class="flabel">Entry fee</span>
        <form id="feeForm" class="row" onsubmit={saveFee}>
          <input id="feeInput" bind:this={feeEl} type="number" inputmode="numeric" min="0" max="50" value={chat.roomFee} aria-label="Entry fee in credits">
          <button type="submit">Save fee</button>
        </form>
      </div>
      <div class="hfield">
        <span class="flabel">Password</span>
        <form id="pwForm" class="row" autocomplete="off" onsubmit={savePw}>
          <input id="pwInput" bind:this={pwEl} type="password" maxlength="32" placeholder={chat.roomPw ? 'New password' : 'Add a password'} autocomplete="new-password" aria-label="Room password">
          <button type="submit">Save</button>
          {#if chat.roomPw}<button type="button" class="ghost" id="pwRemove" onclick={removePw}>Remove</button>{/if}
        </form>
      </div>
      <div class="hfield danger">
        <button type="button" class="warn" id="closeRoom" onclick={close}>{confirmClose ? 'Tap again to close for good' : 'Close room'}</button>
        <p class="hint">Closing deletes the room and everything said in it. Credits spent are not refunded.</p>
      </div>
    </div>
  </details>
{/if}
