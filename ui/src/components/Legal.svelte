<script>
  import { chat, openLegal, closeLegal } from '../lib/chat.svelte.js';
</script>

{#if chat.legal}
  <div class="modal legalmodal" role="presentation" onclick={e => { if (e.target === e.currentTarget) closeLegal(); }}>
    <section id="legalPage" class="pcard legalcard" role="dialog" aria-modal="true" aria-label="Terms and privacy">
      <div class="legalbar">
        <div class="seg" role="tablist">
          <button type="button" role="tab" aria-selected={chat.legal === 'terms'} class={chat.legal === 'terms' ? 'on' : ''} onclick={() => openLegal('terms')}>Terms</button>
          <button type="button" role="tab" aria-selected={chat.legal === 'privacy'} class={chat.legal === 'privacy' ? 'on' : ''} onclick={() => openLegal('privacy')}>Privacy</button>
        </div>
        <button type="button" id="legalClose" class="ghost small" onclick={closeLegal}>Close</button>
      </div>
      <div class="legaltext">
        {#if chat.legal === 'terms'}
          <h2>Terms of use</h2>
          <p class="hint">Last updated October 2026. Plain language, on purpose.</p>
          <h3>Who can use STREETSYNC</h3>
          <p>You must be 16 or older. One person, one account. Keep your password to yourself. You are responsible for what happens under your account.</p>
          <h3>How to behave</h3>
          <p>No hate, threats, harassment, scams, spam, sexual content involving minors, or anything illegal. Do not share other people's private details. Do not try to break, overload or cheat the service or its points and credits.</p>
          <h3>Moderation</h3>
          <p>Staff can delete messages, mute you, remove you from a room or close an account that breaks these rules. Rooms made by users are run by their hosts, who can remove people from their own room. Staff have the last word.</p>
          <h3>Credits and tips</h3>
          <p>Credits are virtual items inside STREETSYNC. They are not money, they pay no interest, and they can not be withdrawn for cash. Credits you spend on tips, gifts or room fees are not refunded unless a mistake was ours. We can change prices and rewards, and we can remove credits gained by cheating.</p>
          <h3>Your content</h3>
          <p>You own what you write. By posting it you let us show it to the people in the room and store it for the time described in the privacy page. Do not post what you do not have the right to share.</p>
          <h3>No guarantees</h3>
          <p>STREETSYNC is provided as it is. It can go down, be slow or lose data. We are not responsible for what other users say or do. If you lose your password there is no reset yet, so write it down.</p>
          <h3>Contact</h3>
          <p>Message the admin, @Eldorado, inside the app.</p>
        {:else}
          <h2>Privacy</h2>
          <p class="hint">Last updated October 2026. Plain language, on purpose.</p>
          <h3>What we keep</h3>
          <p>Your handle, a scrambled (hashed) version of your password, your avatar choice, your points, streak and credits, your friends and blocked list, and your login tokens. We do not ask for your phone number or email.</p>
          <h3>Messages</h3>
          <p>Room messages are kept for 7 days, then deleted. Private messages are kept for 30 days. You can delete your own messages at any time. A message you reported is kept with the report until staff review it.</p>
          <h3>Who sees what</h3>
          <p>Room messages are seen by everyone in the room. Private messages are seen by the two people in the conversation. Staff see messages only when they are reported or when they are working inside a room. Your handle, tier and avatar are public.</p>
          <h3>On your device</h3>
          <p>Your browser stores a login token so you stay signed in. We do not use advertising trackers.</p>
          <h3>Services we use</h3>
          <p>The app runs on Render and keeps its data in a Supabase database. Those companies process data for us and may keep technical logs such as IP addresses.</p>
          <h3>Inactive accounts</h3>
          <p>Accounts without a password that stay idle for 14 days are removed. Logins expire after 30 days without use.</p>
          <h3>Your choices</h3>
          <p>Ask the admin, @Eldorado, in the app to delete your account and its data. This page will change when the product changes, and the date above will change with it.</p>
        {/if}
      </div>
    </section>
  </div>
{/if}
