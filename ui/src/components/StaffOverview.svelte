<script>
  import { chat, setSection, loadSection } from '../lib/chat.svelte.js';
  import Icon from './Icon.svelte';

  let { area } = $props();
  const s = $derived(chat.adm.stats);
  const n = v => (v == null ? '-' : Number(v).toLocaleString());
  const dayName = d => new Date(d + 'T00:00:00Z').toLocaleDateString([], { weekday: 'short', timeZone: 'UTC' });
  const peak = list => Math.max(1, ...(list || []).map(x => x.n));
</script>

<div class="pagehead"><h2>Overview</h2><p>{area === 'admin' ? 'How the platform is doing right now.' : 'What needs your attention.'}</p></div>

{#if !s}
  <p class="hint">Loading the numbers.</p>
{:else}
  <div class="statgrid" id="statGrid">
    {#if area === 'admin' && s.us != null}
      <div class="stat"><span class="k">People</span><b id="stUsers">{n(s.us)}</b><small>{n(s.pw)} with a password</small></div>
      <div class="stat"><span class="k">New in 24 hours</span><b id="stNew">{n(s.n1)}</b><small>{n(s.n7)} in 7 days</small></div>
      <div class="stat"><span class="k">Active in 24 hours</span><b id="stActive">{n(s.a1)}</b><small>logged in or chatted</small></div>
    {/if}
    <div class="stat live"><span class="k">Online now</span><b id="stOnline">{n(s.on)}</b><small>{n(s.ir)} inside rooms</small></div>
    {#if area === 'admin' && s.m1 != null}
      <div class="stat"><span class="k">Messages in 24 hours</span><b id="stMsgs">{n(s.m1)}</b><small>room messages</small></div>
      <div class="stat"><span class="k">Credits held</span><b id="stCredits">{n(s.cr)}</b><small>across all wallets</small></div>
      <div class="stat"><span class="k">Staff</span><b id="stStaff">{n((s.mo || 0) + (s.ad || 0))}</b><small>{n(s.mo)} moderators, {n(s.ad)} admin</small></div>
    {/if}
    <div class="stat"><span class="k">Rooms</span><b id="stRooms">{n(s.ro + s.rc)}</b><small>{n(s.ro)} official, {n(s.rc)} community</small></div>
    <button type="button" class={'stat link' + (s.rep ? ' alert' : '')} id="stReports" onclick={() => setSection(area, 'reports')}><span class="k">Open reports</span><b>{n(s.rep)}</b><small>{s.rep ? 'Review them' : 'All clear'}</small></button>
    <div class="stat"><span class="k">Muted now</span><b id="stMutes">{n(s.mu)}</b><small>people</small></div>
    <div class="stat"><span class="k">Kicked now</span><b id="stKicks">{n(s.ki)}</b><small>people</small></div>
  </div>

  {#if area === 'admin' && s.md}
    <div class="twocol">
      <div class="card pad">
        <h3 class="eyebrow">Messages, last 7 days</h3>
        <div class="bars" id="chartMsgs">
          {#each s.md as d (d.d)}<div class="bar" title={d.d + ': ' + d.n}><i style:height={Math.max(3, Math.round(d.n / peak(s.md) * 100)) + '%'}></i><b>{d.n}</b><span>{dayName(d.d)}</span></div>{/each}
        </div>
      </div>
      <div class="card pad">
        <h3 class="eyebrow">New people, last 7 days</h3>
        <div class="bars" id="chartUsers">
          {#each s.ud as d (d.d)}<div class="bar" title={d.d + ': ' + d.n}><i style:height={Math.max(3, Math.round(d.n / peak(s.ud) * 100)) + '%'}></i><b>{d.n}</b><span>{dayName(d.d)}</span></div>{/each}
        </div>
      </div>
    </div>
  {/if}

  {#if area === 'mod'}
    <div class="card pad" id="permCard">
      <h3 class="eyebrow">What a moderator can do</h3>
      <div class="twocol tight">
        <ul class="plainlist yes">
          <li>Review and settle reports</li>
          <li>Mute or kick people for up to 24 hours</li>
          <li>Delete messages and clear a bio</li>
          <li>Close community rooms</li>
          <li>Enter every room</li>
        </ul>
        <ul class="plainlist no">
          <li>Act on other moderators or admins</li>
          <li>Make or remove moderators</li>
          <li>Give credits or see wallets</li>
          <li>See platform statistics</li>
        </ul>
      </div>
    </div>
  {/if}
{/if}
<div class="row"><button type="button" class="ghost small" id="statsRefresh" onclick={() => loadSection(area)}><Icon name="chart" size={14} /> Refresh</button></div>
