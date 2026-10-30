<script>
  import { chat, signIn, setMode, openLegal } from '../lib/chat.svelte.js';
  import Brand from './Brand.svelte';
  import Avatar from './Avatar.svelte';
  import LookPicker from './LookPicker.svelte';
  import Scene from './Scene.svelte';

  let handleEl, passEl;
  let look = $state('x0'); // the avatar picked while signing up
  let show = $state(false);
  function submit(e) {
    e.preventDefault();
    signIn(handleEl.value, passEl.value, look);
  }
</script>

<div id="landing" hidden={chat.view !== 'join'}>
  <div class="landing-inner">
    <Scene />
    <section class="hero-text">
      <Brand size={34} />
      <h2>Real people.<br>Live conversations.<br><span class="grad">Your crowd, your city.</span></h2>
      <p>Join rooms by interest and city, make friends, send a private message, tip someone for a great take. No phone number. Pick a handle and a password and you are in.</p>
      <ul class="points">
        <li>Rooms for every interest and every city</li>
        <li>Friends and private messages</li>
        <li>Tips: send a suya, a burger or a pizza</li>
        <li>Free to join</li>
      </ul>
    </section>

    <form id="join" autocomplete="on" hidden={chat.view !== 'join'} onsubmit={submit}>
      <label for="handle" id="joinTitle">{chat.loginMode ? 'Log in' : 'Create your account'}</label>
      <p id="joinHint">{chat.loginMode ? 'Use the handle and password you created.' : 'Pick a handle and a password. You need both to log back in.'}</p>
      <input id="handle" bind:this={handleEl} maxlength="17" autocapitalize="off" autocomplete="username" spellcheck="false" placeholder="@yourname">
      <div class="passrow">
        <input id="pass" bind:this={passEl} type={show ? 'text' : 'password'} maxlength="128" placeholder={chat.loginMode ? 'Password' : 'Password (8 or more characters)'} autocomplete={chat.loginMode ? 'current-password' : 'new-password'}>
        <button type="button" id="showPass" class="ghost small" aria-pressed={show} onclick={() => (show = !show)}>{show ? 'Hide' : 'Show'}</button>
      </div>
      {#if !chat.loginMode}
        <p class="hint" id="passHint">Write it down. There is no email reset yet.</p>
        <div id="lookBlock">
          <span class="lookhead">Choose your avatar</span>
          <LookPicker value={look} onpick={c => (look = c)} />
        </div>
      {/if}
      <div class="row"><button id="enter" type="submit" disabled={chat.joinBusy}>{chat.loginMode ? 'Log in' : 'Create account'}</button></div>
      <button type="button" id="toggle" class="link" onclick={() => setMode(!chat.loginMode)}>{chat.loginMode ? 'I am new here' : 'I already have an account'}</button>
      <div id="joinErr" class="err" role="alert">{chat.joinErr}</div>
    </form>

    <section class="preview" aria-label="What a room looks like">
      <div class="pv-head"><b>General Chat</b><small>Example</small></div>
      <div class="pv-msg"><Avatar handle="Amaka" size={30} /><div><b>@Amaka</b><span>Anyone watching the match tonight?</span></div></div>
      <div class="pv-msg"><Avatar handle="Tunde" size={30} /><div><b>@Tunde</b><span>Always. Kickoff is at 8, come through</span></div></div>
      <div class="pv-tip"><span aria-hidden="true">&#127828;</span><span><b>@Amaka</b> sent <b>@Tunde</b> a <b>Burger</b></span><em>5 credits</em></div>
      <div class="pv-msg"><Avatar handle="Zainab" size={30} /><div><b>@Zainab</b><span>Joining from Abuja, what did I miss?</span></div></div>
    </section>

    <p class="legal">STREETSYNC. Free to join, no phone number needed. <button type="button" class="link" id="legalTerms" onclick={() => openLegal('terms')}>Terms</button> &middot; <button type="button" class="link" id="legalPrivacy" onclick={() => openLegal('privacy')}>Privacy</button></p>
  </div>
</div>
