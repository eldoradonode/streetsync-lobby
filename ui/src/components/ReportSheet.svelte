<script>
  import { chat, closeReport, submitReport, toggleDMReportItem } from '../lib/chat.svelte.js';
  const r = $derived(chat.reportSheet);
  const staff = $derived(r?.kind === 'staff');
  const REASONS = [['spam', 'Spam'], ['abuse', 'Abuse'], ['harassment', 'Harassment'], ['scam', 'Scam'], ['other', 'Other']];
  const STAFF_REASONS = [['abuse', 'Abuse of power'], ['harassment', 'Harassment'], ['unfair', 'Unfair action'], ['other', 'Something else']];
  const TITLE = { dm: 'Report private messages', message: 'Report this message', user: 'Report this person', room: 'Report this room', staff: 'Report staff misconduct' };
  const max = $derived(staff ? 500 : 300);
  const choices = $derived(r?.kind === 'dm' ? chat.thread.items.filter(x => x.f.toLowerCase() !== (chat.me || '').toLowerCase() && (x.legacy || !x.err)) : []);
</script>

{#if r}
  <div class="modal" role="presentation" onclick={e => { if (e.target === e.currentTarget) closeReport(); }}>
    <form class="pcard sheetform" id="reportSheet" autocomplete="off" onsubmit={e => { e.preventDefault(); submitReport(); }} aria-label={TITLE[r.kind]}>
      <h3 class="mtitle">{TITLE[r.kind]}</h3>
      {#if r.kind === 'dm'}
        <p class="hint" id="dmReportNote"><b>These messages are private.</b> Reviewers can read only the messages you tick below, and nothing else from this conversation. Messages you do not tick stay unreadable to everyone but you and @{r.h}. Nothing is shared until you press Send report.</p>
        <ul class="pickmsgs" id="pickMsgs">
          {#each choices as m (m.i)}
            <li><label><input type="checkbox" checked={r.ids.includes(m.i)} onchange={() => toggleDMReportItem(m.i)}> <span>{m.b}</span></label></li>
          {/each}
        </ul>
      {/if}
      {#if staff}<p class="hint">This goes to the admins only. Moderators cannot see it, and @{r.h} will not see who sent it.</p>{/if}
      <div class="reasons" role="radiogroup" aria-label="Reason">
        {#each (staff ? STAFF_REASONS : REASONS) as [k, label] (k)}
          <button type="button" role="radio" aria-checked={r.c === k} class={'chipbtn' + (r.c === k ? ' on' : '')} data-reason={k} onclick={() => { r.c = k; r.err = ''; }}>{label}</button>
        {/each}
      </div>
      {#if r.kind !== 'message' && r.kind !== 'dm'}
        <label for="repDetails">What happened? {r.c === 'other' ? '(required)' : '(optional)'}</label>
        <textarea id="repDetails" bind:value={r.d} rows="3" maxlength={max}></textarea>
        <small class="hint">{max - r.d.length} characters left</small>
      {/if}
      {#if r.err}<p class="errtext" id="repErr">{r.err}</p>{/if}
      <div class="row">
        <button type="submit" id="repSend" disabled={r.pending}>{r.pending ? 'Sending' : 'Send report'}</button>
        <button type="button" class="ghost" onclick={closeReport}>Cancel</button>
      </div>
    </form>
  </div>
{/if}
