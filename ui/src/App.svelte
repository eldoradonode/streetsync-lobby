<script>
  import Join from './components/Join.svelte';
  import Hub from './components/Hub.svelte';
  import Room from './components/Room.svelte';
  import Profile from './components/Profile.svelte';
  import Inbox from './components/Inbox.svelte';
  import Thread from './components/Thread.svelte';
  import Icon from './components/Icon.svelte';
  import Brand from './components/Brand.svelte';
  import Avatar from './components/Avatar.svelte';
  import UserProfile from './components/UserProfile.svelte';
  import Rooms from './components/Rooms.svelte';
  import CreateRoom from './components/CreateRoom.svelte';
  import Legal from './components/Legal.svelte';
  import Wallet from './components/Wallet.svelte';
  import StaffArea from './components/StaffArea.svelte';
  import { SECTIONS, AREA_NAME } from './lib/areas.js';
  import { chat, go, leaveRoom, closeThread, closeUser, openUser, isMod, setSection } from './lib/chat.svelte.js';

  const area = $derived(chat.view === 'admin' || chat.view === 'mod' ? chat.view : '');
  const title = $derived(chat.view === 'room' ? chat.roomName || 'Room' : chat.view === 'hub' ? 'Home' : chat.view === 'rooms' ? 'Rooms' : chat.view === 'profile' ? 'Profile' : chat.view === 'wallet' ? 'Wallet' : area ? AREA_NAME[area] + ' / ' + SECTIONS[area].find(x => x.id === chat.section[area])?.label : chat.view === 'inbox' ? 'Messages' : chat.view === 'thread' ? '@' + chat.thread.h : chat.view === 'user' ? '@' + chat.userView.h : 'STREETSYNC');
  const hasBack = $derived(chat.view === 'room' || chat.view === 'thread' || chat.view === 'user');
  const back = () => (chat.view === 'thread' ? closeThread() : chat.view === 'user' ? closeUser() : leaveRoom());
</script>

<div id="shell" data-area={area || undefined} class={(chat.view === 'room' || chat.view === 'thread' ? 'inroom' : '') + (hasBack ? ' hasback' : '') + (chat.view === 'join' ? ' joining' : '') + (area ? ' inarea' : '')}>
<nav id="tabs" hidden={chat.view === 'join' || chat.view === 'blocked'} aria-label={area ? AREA_NAME[area] : 'Main'}>
  {#if area}
    <div class="side-brand"><Brand /><span class="areatag" id="areaTag">{area === 'admin' ? 'ADMIN CONSOLE' : 'MODERATION'}</span></div>
    {#each SECTIONS[area] as sec (sec.id)}
      <button type="button" id={'sec-' + sec.id} class={chat.section[area] === sec.id ? 'active' : ''} onclick={() => setSection(area, sec.id)}><Icon name={sec.icon} /><span>{sec.label}</span>{#if sec.id === 'reports' && chat.reportCount > 0}<span class="badge">{chat.reportCount}</span>{/if}</button>
    {/each}
    <button type="button" id="areaExit" class="exit" onclick={() => go('hub')}><Icon name="back" /><span>Back to the app</span></button>
  {:else}
    <div class="side-brand"><Brand /></div>
    <button type="button" id="tabHub" class={chat.view === 'hub' ? 'active' : ''} onclick={() => go('hub')}><Icon name="home" /><span>Home</span></button>
    <button type="button" id="tabRooms" class={chat.view === 'rooms' ? 'active' : ''} onclick={() => go('rooms')}><Icon name="rooms" /><span>Rooms</span></button>
    <button type="button" id="tabInbox" class={chat.view === 'inbox' ? 'active' : ''} onclick={() => go('inbox')}><Icon name="chat" /><span>Messages</span>{#if chat.dmUnread + chat.requests > 0}<span class="badge" id="inboxBadge">{chat.dmUnread + chat.requests}</span>{/if}</button>
    <button type="button" id="tabWallet" class={chat.view === 'wallet' ? 'active' : ''} onclick={() => go('wallet')}><Icon name="wallet" /><span>Wallet</span></button>
    <button type="button" id="tabProfile" class={chat.view === 'profile' ? 'active' : ''} onclick={() => go('profile')}><Icon name="user" /><span>Profile</span>{#if isMod() && chat.reportCount > 0}<span class="badge">{chat.reportCount}</span>{/if}</button>
    {#if isMod()}
      <div class="staffnav" id="staffNav">
        <span class="navlabel">Staff</span>
        {#if chat.role === 'admin'}<button type="button" id="tabAdmin" class="staffbtn admin" onclick={() => go('admin')}><Icon name="shield" /><span>Admin console</span></button>{/if}
        <button type="button" id="tabMod" class="staffbtn mod" onclick={() => go('mod')}><Icon name="tools" /><span>Moderation</span>{#if chat.reportCount > 0}<span class="badge">{chat.reportCount}</span>{/if}</button>
      </div>
    {/if}
  {/if}
</nav>
<main class="screen">
  <header>
    <div class="hl">
      <button type="button" id="back" class="ghost" hidden={!hasBack} aria-label={chat.view === 'room' ? 'Back to the hub' : 'Back'} onclick={back}>&#8592;</button>
      {#if chat.view === 'thread'}
        <button type="button" id="titleUser" class="titlebtn" aria-label={'Open the profile of ' + chat.thread.h} onclick={() => openUser(chat.thread.h)}>
          <Avatar handle={chat.thread.h} size={30} ring="transparent" />
          <h1 id="title">{title}</h1>
        </button>
      {:else}
        <h1 id="title">{title}</h1>
      {/if}
    </div>
    <button type="button" class="chip" id="credChip" hidden={chat.view !== 'hub' && chat.view !== 'profile' && chat.view !== 'inbox' && chat.view !== 'rooms' && chat.view !== 'wallet'} aria-label="Your credits. Open the wallet." onclick={() => go('wallet')}>&#9670; <span id="credCount">{chat.credits.toLocaleString()}</span></button>
    <span class="online" id="online" hidden={chat.view !== 'room'}><span class="dot" aria-hidden="true"></span><span><span id="count">{chat.count}</span> online</span></span>
  </header>
  <div id="status" role="status" hidden={!chat.status}>{chat.status}</div>

  <Join />

  <section id="blocked" hidden={chat.view !== 'blocked'}>
    <b>You were removed from STREETSYNC</b>
    <span id="blockedMsg">{chat.blockedMsg}</span>
  </section>

  <section id="app" hidden={chat.view === 'join' || chat.view === 'blocked'}>
    <Hub />
    <Rooms />
    <Inbox />
    <Profile />
    <Wallet />
    <StaffArea />
    <Room />
    <Thread />
    <UserProfile />
  </section>
</main>
{#if chat.createOpen && chat.view !== 'join'}<CreateRoom onclose={() => (chat.createOpen = false)} />{/if}
<Legal />
</div>
