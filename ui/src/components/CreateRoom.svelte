<script>
  import { chat, createRoom, ROOM_CATEGORIES } from '../lib/chat.svelte.js';

  let { onclose } = $props();
  let name, kind, tags, fee, pw, cat;
  let type = $state('public');

  const TYPES = [
    ['public', 'Public', 'Anyone can find it on the hub'],
    ['private', 'Private', 'Only people with your invite code'],
    ['password', 'Password', 'Unlisted. People need your password'],
    ['paid', 'Paid', 'People pay credits once to enter'],
  ];
  function pick(t) {
    type = t;
    kind.value = t === 'private' ? 'private' : 'public';
    if (t !== 'password') pw.value = '';
    if (t === 'paid') { if (!Number(fee.value)) fee.value = '5'; } else fee.value = '0';
    if (t === 'password') pw.focus();
    if (t === 'paid') fee.focus();
  }
  function submit(e) {
    e.preventDefault();
    if (createRoom({ name: name.value, kind: kind.value, category: cat.value, tags: tags.value, fee: fee.value, pw: pw.value })) onclose();
  }
</script>

<div class="modal" role="presentation" onclick={e => { if (e.target === e.currentTarget) onclose(); }}>
  <form id="createRoom" class="pcard" autocomplete="off" onsubmit={submit} aria-label="Create a room">
    <h3 class="mtitle">Create a room</h3>
    <div class="typecards" role="radiogroup" aria-label="Room type">
      {#each TYPES as [k, label, hint] (k)}
        <button type="button" role="radio" aria-checked={type === k} class={'typecard' + (type === k ? ' sel' : '')} data-type={k} onclick={() => pick(k)}><b>{label}</b><small>{hint}</small></button>
      {/each}
    </div>
    <label for="crName">Room name</label>
    <input id="crName" bind:this={name} maxlength="30" placeholder="Abuja Night Owls">
    <label for="crKind" hidden>Who can find it</label>
    <select id="crKind" bind:this={kind} hidden>
      <option value="public">Public</option>
      <option value="private">Private</option>
    </select>
    <label for="crCat">Category</label>
    <select id="crCat" bind:this={cat}>
      {#each ROOM_CATEGORIES as c}<option value={c}>{c}</option>{/each}
    </select>
    <label for="crTags">Tags (up to 3, separated by commas)</label>
    <input id="crTags" bind:this={tags} maxlength="60" placeholder="Gist, Night, Abuja">
    <div class="field" hidden={type !== 'paid'}>
      <label for="crFee">Entry fee in credits (0 to 50, you keep 80%)</label>
      <input id="crFee" bind:this={fee} type="number" inputmode="numeric" min="0" max="50" value="0">
    </div>
    <div class="field" hidden={type !== 'password'}>
      <label for="crPw">Room password (4 to 32 characters)</label>
      <input id="crPw" bind:this={pw} type="password" maxlength="32" autocomplete="new-password">
    </div>
    <p class="hint" id="crNote">Costs 25 credits. You have {chat.credits.toLocaleString()}. Needs Regular. One room each, two for Legends. Closes by itself after 3 days without messages.</p>
    <div class="row">
      <button type="submit" id="crGo">Create room</button>
      <button type="button" class="ghost" onclick={onclose}>Cancel</button>
    </div>
  </form>
</div>
