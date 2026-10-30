<script>
  import PasswordField from './PasswordField.svelte';
  import { chat, useThisDevice, setPref, setRoomNotices, saveBio, changePassword, openLegal, go, openSecure, deleteStart, deleteCancel, deleteVerify, deleteFinal } from '../lib/chat.svelte.js';

  let bioText = $state('');
  let editing = $state(false);
  let oldPw = $state('');
  let newPw = $state('');
  const left = $derived(160 - [...bioText].length);

  const startBio = () => { bioText = chat.bio; editing = true; };
  function submitBio(e) {
    e.preventDefault();
    if (saveBio(bioText)) editing = false;
  }
  function submitPw(e) {
    e.preventDefault();
    if (changePassword(oldPw, newPw)) { oldPw = ''; newPw = ''; }
  }

  let keyPw = $state('');
  const fpShort = fp => (fp ? fp.slice(0, 32).replace(/(.{4})/g, '$1 ').trim() : '');
  let delPw = $state('');
  let delHandle = $state('');
  let delWord = $state('');

  const TOGGLES = [
    ['nd', 'Direct messages', 'A notification and a pop up when someone sends you a message. Your unread count and inbox always work.'],
    ['nk', 'Tags and replies', 'When someone tags you with @ or replies to you in a room.'],
    ['nt', 'Gifts', 'When someone sends you a tip or a gift.'],
    ['nf', 'Friend requests', 'When someone asks to be your friend, or accepts.'],
  ];
</script>

<section id="settingsPage" hidden={chat.view !== 'settings'}>
  <div class="pagehead"><h2>Settings</h2><p>Only things that change how STREETSYNC treats you.</p></div>

  <div class="card pad setgroup" id="setProfile">
    <h3 class="eyebrow">Profile</h3>
    {#if editing}
      <form id="setBioForm" autocomplete="off" onsubmit={submitBio}>
        <label for="setBio">Bio</label>
        <textarea id="setBio" bind:value={bioText} rows="3" maxlength="200" placeholder="A line or two about you."></textarea>
        <div class="row between">
          <small class={'hint' + (left < 0 ? ' warn-text' : '')} id="setBioLeft">{left} characters left</small>
          <span class="row">
            <button type="submit" id="setBioSave" disabled={left < 0}>Save</button>
            <button type="button" class="ghost" onclick={() => (editing = false)}>Cancel</button>
          </span>
        </div>
      </form>
    {:else}
      <p id="setBioText" class={'bio' + (chat.bio ? '' : ' none')}>{chat.bio || 'No bio yet.'}</p>
      <div class="row wrap">
        <button type="button" id="setBioEdit" class="ghost small" onclick={startBio}>{chat.bio ? 'Edit bio' : 'Add a bio'}</button>
        <button type="button" id="setAvatar" class="ghost small" onclick={() => go('profile')}>Change avatar</button>
      </div>
    {/if}
  </div>

  <div class="card pad setgroup" id="setNotes">
    <h3 class="eyebrow">Notifications</h3>
    {#each TOGGLES as [key, label, hint] (key)}
      <label class="switchrow" for={'sw-' + key}>
        <span><b>{label}</b><small>{hint}</small></span>
        <input id={'sw-' + key} type="checkbox" role="switch" checked={chat.prefs[key]} onchange={e => setPref(key, e.currentTarget.checked)}>
      </label>
    {/each}
    <label class="switchrow" for="sw-roomNotices">
      <span><b>Room join and leave lines</b><small>Show "@name joined the room" and "@name left the room" while you chat. Only on this device.</small></span>
      <input id="sw-roomNotices" type="checkbox" role="switch" checked={chat.roomNotices} onchange={e => setRoomNotices(e.currentTarget.checked)}>
    </label>
    <p class="hint">Turning one off stops new notifications of that kind. Messages still arrive in your inbox. Safety and moderation notices are always sent. Notifications appear inside STREETSYNC only; there are no push messages to your phone yet.</p>
  </div>

  <div class="card pad setgroup" id="setPrivacy">
    <h3 class="eyebrow">Privacy</h3>
    <label class="switchrow" for="setFriendsOnly">
      <span><b>Only friends can message me</b><small>Everyone else is told you only take messages from friends. Moderators can still reach you.</small></span>
      <input id="setFriendsOnly" type="checkbox" role="switch" checked={chat.prefs.dp === 'friends'} onchange={e => setPref('dp', e.currentTarget.checked ? 'friends' : 'everyone')}>
    </label>
    <label class="switchrow" for="setShowOnline">
      <span><b>Show when I am online</b><small>On your profile and under your name in a conversation. Moderators can always see it on your profile.</small></span>
      <input id="setShowOnline" type="checkbox" role="switch" checked={chat.prefs.so} onchange={e => setPref('so', e.currentTarget.checked)}>
    </label>
  </div>

  <div class="card pad setgroup" id="setEncrypt">
    <h3 class="eyebrow">Encrypted messages</h3>
    {#if chat.e2e.state === 'unsupported'}
      <p class="hint warn-text">This browser cannot encrypt messages, so direct messages are off here. Use an up to date browser.</p>
    {:else if chat.e2e.state === 'ready'}
      <p class="hint" id="encReady"><b>On.</b> New direct messages are encrypted on your device before they are sent. STREETSYNC, its database and its hosts cannot read them.</p>
      <p class="hint">Your key fingerprint: <code id="myFp">{fpShort(chat.e2e.fp)}</code></p>
    {:else if chat.e2e.state === 'mismatch' || chat.e2e.state === 'other_device'}
      <p class="hint warn-text" id="encOther"><b>Your key is on another device.</b> This device cannot read or send encrypted messages. You can make a new key here. Messages made for the old key stay readable only on the device that holds it, and your contacts will see a warning that your key changed.</p>
      <form id="encUseForm" autocomplete="off" onsubmit={e => { e.preventDefault(); useThisDevice(keyPw); keyPw = ''; }}>
        {#if chat.hasPw}<PasswordField id="encPw" label="Your password" bind:value={keyPw} autocomplete="current-password" />{/if}
        {#if chat.e2e.err}<p class="errtext" id="encErr">{chat.e2e.err}</p>{/if}
        <div class="row"><button type="submit" id="encUse" disabled={chat.e2e.busy}>Use this device</button></div>
      </form>
    {:else}
      <p class="hint">Setting up your encryption key.</p>
    {/if}
    {#if chat.e2e.persistent === false}<p class="hint warn-text" id="encVolatile">This browser is not storing your key. It will be lost when you close the page, and you will not be able to read these messages again.</p>{/if}
    <p class="hint"><b>What to know.</b> Your key lives only on this device. Only one device can read your messages. If you clear this browser's data or lose the device, those messages are gone for good, and no one, including us, can recover them. A password reset never gives anyone access to them. Reporting a message shares that one message with the reviewers, and only if you choose to. Messages are deleted after 7 days. This feature has not had an independent security review.</p>
  </div>

  <div class="card pad setgroup" id="setSecurity">
    <h3 class="eyebrow">Security</h3>
    {#if chat.hasPw}
      <form id="setPwForm" autocomplete="off" onsubmit={submitPw}>
        <PasswordField id="setOldPw" label="Current password" bind:value={oldPw} autocomplete="current-password" />
        <PasswordField id="setNewPw" label="New password (8 or more characters)" bind:value={newPw} autocomplete="new-password" />
        <div class="row"><button type="submit" id="setPwSave">Change password</button></div>
        <p class="hint">Changing it logs you out on your other devices.</p>
      </form>
    {:else}
      <p class="hint">Your handle has no password. Without one you can never log back in.</p>
      <div class="row"><button type="button" id="setSecure" onclick={() => { go('profile'); openSecure(); }}>Protect my handle</button></div>
    {/if}
    <p class="hint legalrow"><button type="button" class="link" onclick={() => openLegal('terms')}>Terms</button> &middot; <button type="button" class="link" onclick={() => openLegal('privacy')}>Privacy</button></p>
  </div>

  <div class="card pad setgroup" id="setHelp">
    <h3 class="eyebrow">Help</h3>
    <p class="hint"><b>Report someone or something:</b> open a message, a profile or a room and choose Report.</p>
    <p class="hint"><b>Report an admin or moderator:</b> choose "Report staff misconduct" on their message or profile. Only admins can read these, and moderators never see them.</p>
    <p class="hint"><b>Admins and moderators</b> cannot be tipped, messaged first or sent friend requests. Reach them through a report.</p>
  </div>

  <div class="card pad setgroup dangerzone" id="setDelete">
    <h3 class="eyebrow">Delete account</h3>
    {#if chat.del.stage === 0}
      <p class="hint">Permanently delete your account. This cannot be undone.</p>
      <div class="row"><button type="button" class="warn" id="delStart" onclick={deleteStart}>Delete my account</button></div>
    {:else if chat.del.stage === 1}
      <p><b>What happens:</b></p>
      <ul class="hint">
        <li>You are logged out everywhere, immediately. Your handle is released.</li>
        <li>Your messages, direct messages, friends, blocks, notifications, settings and profile are deleted.</li>
        <li>Community rooms you host are closed with their messages, and their members are told.</li>
        <li>Any credits you have are forfeited. They cannot be refunded or moved.</li>
        <li>Credit records stay, with no name or password, under an anonymous number. Reports about you and reports you filed stay for safety, with your handle removed.</li>
      </ul>
      <form id="delVerify" autocomplete="off" onsubmit={e => { e.preventDefault(); deleteVerify(delPw, delHandle); }}>
        {#if chat.hasPw}
          <PasswordField id="delPw" label="Enter your password to continue" bind:value={delPw} autocomplete="current-password" />
        {:else}
          <label for="delHandle">Type your handle (@{chat.me}) to continue</label>
          <input id="delHandle" bind:value={delHandle} maxlength="17" autocapitalize="off">
        {/if}
        {#if chat.del.err}<p class="errtext" id="delErr">{chat.del.err}</p>{/if}
        <div class="row"><button type="submit" class="warn" id="delNext" disabled={chat.del.busy}>Continue</button><button type="button" class="ghost" onclick={deleteCancel}>Cancel</button></div>
      </form>
    {:else}
      <form id="delFinal" autocomplete="off" onsubmit={e => { e.preventDefault(); deleteFinal(delWord); }}>
        <p><b>Last step.</b> Type DELETE to delete @{chat.me} for good. This works for 5 minutes.</p>
        <input id="delWord" bind:value={delWord} maxlength="10" autocapitalize="characters" aria-label="Type DELETE">
        {#if chat.del.err}<p class="errtext" id="delErr">{chat.del.err}</p>{/if}
        <div class="row"><button type="submit" class="warn" id="delGo" disabled={chat.del.busy || delWord !== 'DELETE'}>Delete my account now</button><button type="button" class="ghost" onclick={deleteCancel}>Cancel</button></div>
      </form>
    {/if}
  </div>
</section>
