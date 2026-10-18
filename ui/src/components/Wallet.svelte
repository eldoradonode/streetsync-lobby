<script>
  import { chat } from '../lib/chat.svelte.js';
  import Icon from './Icon.svelte';

  let filter = $state('all'); // all, in or out
  const when = ts => new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const day = ts => new Date(ts).toLocaleDateString([], { day: 'numeric', month: 'short' });
  const KIND = {
    welcome: 'Welcome gift', daily: 'Daily bonus', streak: 'Streak bonus', grant: 'Gift', dm: 'Message',
    tip_sent: 'Tip sent', tip_received: 'Tip received', room_create: 'Room created', room_entry: 'Room entry', room_income: 'Room income',
  };
  const sign = n => (n > 0 ? '+' : '') + n.toLocaleString();
  const rows = $derived(chat.wallet.filter(e => filter === 'all' || (filter === 'in' ? e.a > 0 : e.a < 0)));
  const earned = $derived(chat.wallet.filter(e => e.a > 0).reduce((n, e) => n + e.a, 0));
  const spent = $derived(chat.wallet.filter(e => e.a < 0).reduce((n, e) => n - e.a, 0));
</script>

<section id="walletPage" hidden={chat.view !== 'wallet'}>
  <div class="pagehead">
    <h2>Wallet</h2>
    <p>Your credits and where they went.</p>
  </div>

  <div id="wallet" class="card balancecard">
    <div class="wlabel"><Icon name="wallet" size={18} /> Balance</div>
    <div class="wbal"><span id="walletBal">{chat.credits.toLocaleString()}</span> <small>credits</small></div>
    <div class="wsplit" hidden={chat.wallet.length === 0}>
      <span><b class="plus">+{earned.toLocaleString()}</b> earned</span>
      <span><b class="minus">-{spent.toLocaleString()}</b> spent</span>
      <small>in your latest activity</small>
    </div>
  </div>

  <div class="twocol">
    <div class="card pad">
      <h3 class="eyebrow">Ways to earn</h3>
      <ul class="plainlist">
        <li>2 credits on your first message each day</li>
        <li>A bonus when your streak reaches 3, 7, 14 and 30 days</li>
        <li>Tips from people in rooms, and entry fees from rooms you host</li>
      </ul>
      <p class="hint">Being online earns nothing. Only chatting counts.</p>
    </div>
    <div class="card pad">
      <h3 class="eyebrow">Ways to spend</h3>
      <ul class="plainlist">
        <li>Send a tip in a room</li>
        <li>1 credit to message someone who is not your friend</li>
        <li>25 credits to create a room</li>
        <li>Entry fees for paid rooms</li>
      </ul>
    </div>
  </div>

  <div class="sechead"><h3>Activity</h3>
    <div class="chips" role="group" aria-label="Filter activity">
      <button type="button" class={'chipbtn' + (filter === 'all' ? ' on' : '')} aria-pressed={filter === 'all'} onclick={() => (filter = 'all')}>All</button>
      <button type="button" class={'chipbtn' + (filter === 'in' ? ' on' : '')} aria-pressed={filter === 'in'} onclick={() => (filter = 'in')}>Earned</button>
      <button type="button" class={'chipbtn' + (filter === 'out' ? ' on' : '')} aria-pressed={filter === 'out'} onclick={() => (filter = 'out')}>Spent</button>
    </div>
  </div>
  <ul id="walletList" class="card ledger">
    {#if chat.wallet.length === 0}
      <li class="empty">Nothing here yet.</li>
    {:else if rows.length === 0}
      <li class="empty">Nothing in this filter.</li>
    {/if}
    {#each rows as e (e.i)}
      <li>
        <span class="wnote"><span class="kind-row"><span class={'kind k-' + e.k}>{KIND[e.k] || e.k}</span>{e.n || ''}</span><small>{day(e.ts)} {when(e.ts)}</small></span>
        <b class={e.a > 0 ? 'plus' : 'minus'}>{sign(e.a)}</b>
      </li>
    {/each}
  </ul>
</section>
