<script>
  import Avatar from './Avatar.svelte';
  import LookPicker from './LookPicker.svelte';
  import Icon from './Icon.svelte';
  import { ensureStaff } from '../lib/staff-loader.svelte.js';
  import { chat, isMod, openSecure, closeSecure, setPassword, logout, setLook, openLegal, saveBio, go } from '../lib/chat.svelte.js';
  import { TIERS, ABBR, STARTS } from '../lib/rank.js';
  import { colorFor } from '../lib/avatar.js';

  let rulesOpen = $state(false);
  let lookOpen = $state(false);
  let bioOpen = $state(false);
  let bioText = $state('');
  let confirmLogout = $state(false); // a handle without a password is lost for good when its owner logs out
  let passEl;

  const staff = $derived(isMod());
  const ring = $derived(chat.role === 'admin' ? '#FB7185' : chat.role === 'mod' ? '#2DD4BF' : 'var(--t' + chat.tier + ')');
  const pct = $derived(chat.next ? Math.min(100, Math.round((chat.points - STARTS[chat.tier]) / (chat.next - STARTS[chat.tier]) * 100)) : 100);
  const joined = $derived(chat.myJoined ? new Date(chat.myJoined).toLocaleDateString([], { month: 'long', year: 'numeric' }) : '');
  const left = $derived(160 - [...bioText].length);

  const startBio = () => { bioText = chat.bio; bioOpen = true; };
  function submitBio(e) {
    e.preventDefault();
    if (saveBio(bioText)) bioOpen = false;
  }
  function savePassword(e) {
    e.preventDefault();
    setPassword(passEl.value);
  }
  const startLogout = () => (chat.hasPw ? logout() : (confirmLogout = true));
</script>

<section id="profile" hidden={chat.view !== 'profile'}>
  <div class="card herocard">
    <div class="banner" aria-hidden="true"></div>
    <div class="herobody">
      <div class="herotop">
        <Avatar handle={chat.me || '?'} {ring} size={88} />
        <div class="heroid">
          <div class="pname">
            <span id="myBadge" class={'bd ' + (staff ? 'r-' + chat.role : 't' + chat.tier)}>{staff ? chat.role.toUpperCase() : ABBR[chat.tier]}</span>
            <b id="myName" style:color={staff ? null : colorFor(chat.me || '?')} class={staff ? 'staffname r-' + chat.role : ''}>@{chat.me}</b>
          </div>
          <p class="hint" id="myJoined" hidden={!joined}>Member since {joined}</p>
        </div>
        <button type="button" id="profSettings" class="ghost small" onclick={() => go('settings')}><Icon name="gear" size={16} /> Settings</button>
        <button type="button" id="lookToggle" class="ghost small" aria-expanded={lookOpen} onclick={() => (lookOpen = !lookOpen)}>{lookOpen ? 'Done' : 'Change avatar'}</button>
      </div>
      {#if lookOpen}<LookPicker value={chat.myLook} onpick={setLook} />{/if}

      {#if bioOpen}
        <form id="bioForm" class="bioform" autocomplete="off" onsubmit={submitBio}>
          <label for="bioInput">About you</label>
          <textarea id="bioInput" bind:value={bioText} rows="3" maxlength="200" placeholder="A line or two about you. What do you chat about?"></textarea>
          <div class="row between">
            <small class={'hint' + (left < 0 ? ' warn-text' : '')} id="bioLeft">{left} characters left</small>
            <span class="row">
              <button type="submit" id="bioSave" disabled={left < 0}>Save</button>
              <button type="button" id="bioCancel" class="ghost" onclick={() => (bioOpen = false)}>Cancel</button>
            </span>
          </div>
        </form>
      {:else}
        <p id="bioText" class={'bio' + (chat.bio ? '' : ' none')}>{chat.bio || 'No bio yet. Tell people who you are.'}</p>
        <button type="button" id="bioEdit" class="link" onclick={startBio}>{chat.bio ? 'Edit bio' : 'Add a bio'}</button>
      {/if}

      <p class="hint" id="staffNote" hidden={!staff}>{chat.role === 'admin' ? 'Admin' : 'Moderator'} accounts can enter every room and do not earn a tier.</p>
      <dl class="statrow" hidden={staff}>
        <div><dt>Tier</dt><dd>{TIERS[chat.tier]}</dd></div>
        <div><dt>Points</dt><dd>{chat.points.toLocaleString()}</dd></div>
        <div><dt>Streak</dt><dd>{chat.streak ? chat.streak + (chat.streak === 1 ? ' day' : ' days') : 'None yet'}</dd></div>
        <div><dt>Credits</dt><dd><button type="button" class="link" id="profCredits" onclick={() => go('wallet')}>{chat.credits.toLocaleString()}</button></dd></div>
      </dl>
      <button type="button" id="myPts" class="pts" hidden={staff} aria-expanded={rulesOpen} aria-controls="rules"
        aria-label="Your points. Tap to see how points work." onclick={() => (rulesOpen = !rulesOpen)}>{TIERS[chat.tier]} · {chat.points}{chat.next ? ' / ' + chat.next : ''} pts{chat.streak ? ' · day ' + chat.streak : ''}</button>
      <div id="bar" hidden={staff} aria-hidden="true"><div id="fill" style:width={pct + '%'} style:background={'var(--t' + chat.tier + ')'}></div></div>
    </div>
  </div>

  <div id="rules" class="card pad" hidden={!rulesOpen || staff}>
    <ul>
      <li><strong>Send real messages to earn points.</strong> Each message of 3 or more characters is worth 1 point.</li>
      <li><strong>Being online earns nothing.</strong> Only chatting counts.</li>
      <li>The same text again within 30 seconds earns nothing, and message points stop at 60 an hour.</li>
      <li><strong>Daily streak:</strong> your first message each day adds a bonus of 10 points for every day in your streak, up to 70.</li>
    </ul>
  </div>

  <div id="tiers" hidden={staff}>
    {#each TIERS as name, i}
      <span class={i === chat.tier ? 'now' : ''}><span class={'bd t' + i}>{ABBR[i]}</span>{name} {STARTS[i].toLocaleString()}</span>
    {/each}
  </div>

  <div id="staffCard" class={'card pad staffcard r-' + chat.role} hidden={!staff}>
    <div>
      <h3 class="eyebrow">Staff tools</h3>
      <p class="hint">{chat.role === 'admin' ? 'You can manage users, moderators and rooms, and review reports.' : 'You can review reports and act on users and rooms.'}</p>
    </div>
    <div class="row wrap">
      {#if chat.role === 'admin'}<button type="button" id="staffAdmin" class="staffbtn admin" onpointerenter={ensureStaff} ontouchstart={ensureStaff} onclick={() => go('admin')}><Icon name="shield" size={18} /> Admin console</button>{/if}
      <button type="button" id="staffMod" class="staffbtn mod" onpointerenter={ensureStaff} ontouchstart={ensureStaff} onclick={() => go('mod')}><Icon name="tools" size={18} /> Moderation{chat.reportCount ? ' (' + chat.reportCount + ')' : ''}</button>
    </div>
  </div>

  <div class="card pad" hidden={chat.hasPw}>
    <button type="button" id="secureBtn" class="link" onclick={openSecure}>Protect my handle with a password</button>
    <p class="hint">Without a password you can never log back in once you log out.</p>
  </div>
  <form id="secure" class="card pad" autocomplete="off" hidden={!chat.secureOpen} onsubmit={savePassword}>
    <label for="newpass">Choose a password (8 or more characters)</label>
    <div class="row">
      <input id="newpass" bind:this={passEl} type="password" maxlength="128" autocomplete="new-password">
      <button type="submit">Save</button>
      <button type="button" id="secureCancel" class="ghost" onclick={closeSecure}>Cancel</button>
    </div>
    <div id="secureErr" class="err" role="alert">{chat.secureErr}</div>
  </form>

  <div class="card pad" id="logoutBox">
    {#if confirmLogout}
      <p id="logoutWarn" class="hint warn-text">@{chat.me} has no password. If you log out, you can not get back into it or its credits.</p>
      <div class="row wrap">
        <button type="button" id="logoutProtect" onclick={() => { confirmLogout = false; openSecure(); }}>Protect it with a password</button>
        <button type="button" id="logoutAnyway" class="warn" onclick={logout}>Log out anyway</button>
        <button type="button" id="logoutCancel" class="ghost" onclick={() => (confirmLogout = false)}>Cancel</button>
      </div>
    {:else}
      <button type="button" id="logout" class="ghost" onclick={startLogout}>Log out</button>
    {/if}
  </div>
  <p class="legalrow hint"><button type="button" class="link" id="profTerms" onclick={() => openLegal('terms')}>Terms</button> &middot; <button type="button" class="link" id="profPrivacy" onclick={() => openLegal('privacy')}>Privacy</button></p>
</section>
