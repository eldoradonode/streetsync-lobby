<script>
  import { chat, setPref, saveBio, changePassword, openLegal, go, openSecure } from '../lib/chat.svelte.js';

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

  const TOGGLES = [
    ['nd', 'Direct messages', 'A notification when someone sends you a message.'],
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
    <p class="hint">Turning one off stops new notifications of that kind. Messages still arrive in your inbox.</p>
  </div>

  <div class="card pad setgroup" id="setPrivacy">
    <h3 class="eyebrow">Privacy</h3>
    <label class="switchrow" for="setFriendsOnly">
      <span><b>Only friends can message me</b><small>Everyone else is told you only take messages from friends. Moderators can still reach you.</small></span>
      <input id="setFriendsOnly" type="checkbox" role="switch" checked={chat.prefs.dp === 'friends'} onchange={e => setPref('dp', e.currentTarget.checked ? 'friends' : 'everyone')}>
    </label>
    <label class="switchrow" for="setShowOnline">
      <span><b>Show when I am online</b><small>On your profile. Moderators can always see it.</small></span>
      <input id="setShowOnline" type="checkbox" role="switch" checked={chat.prefs.so} onchange={e => setPref('so', e.currentTarget.checked)}>
    </label>
  </div>

  <div class="card pad setgroup" id="setSecurity">
    <h3 class="eyebrow">Security</h3>
    {#if chat.hasPw}
      <form id="setPwForm" autocomplete="off" onsubmit={submitPw}>
        <label for="setOldPw">Current password</label>
        <input id="setOldPw" type="password" bind:value={oldPw} maxlength="128" autocomplete="current-password">
        <label for="setNewPw">New password (8 or more characters)</label>
        <input id="setNewPw" type="password" bind:value={newPw} maxlength="128" autocomplete="new-password">
        <div class="row"><button type="submit" id="setPwSave">Change password</button></div>
        <p class="hint">Changing it logs you out on your other devices.</p>
      </form>
    {:else}
      <p class="hint">Your handle has no password. Without one you can never log back in.</p>
      <div class="row"><button type="button" id="setSecure" onclick={() => { go('profile'); openSecure(); }}>Protect my handle</button></div>
    {/if}
    <p class="hint legalrow"><button type="button" class="link" onclick={() => openLegal('terms')}>Terms</button> &middot; <button type="button" class="link" onclick={() => openLegal('privacy')}>Privacy</button></p>
  </div>
</section>
