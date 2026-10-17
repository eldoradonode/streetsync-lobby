<script>
  import { chat, pickTip, sendTip, closeTip } from '../lib/chat.svelte.js';
  import { TIPS, tipFor } from '../lib/tips.js';

  const s = $derived(chat.tipSheet);
  const picked = $derived(s && s.sel ? tipFor(s.sel) : null);
  const plural = n => n + ' credit' + (n === 1 ? '' : 's');
</script>

{#if s}
  <div id="tipScrim" onclick={closeTip} role="presentation"></div>
  <div id="tipSheet" role="dialog" aria-modal="true" aria-label={'Tip @' + s.h}>
    <h3>Tip @{s.h}</h3>
    <p class="hint" id="tipBal">Treats are paid from your credits and go straight to them. You have {plural(chat.credits)}.</p>
    <div id="tipChoices" role="radiogroup" aria-label="Choose a treat">
      {#each TIPS as t (t.a)}
        <button type="button" class={'tipopt' + (s.sel === t.a ? ' sel' : '')} role="radio" aria-checked={s.sel === t.a}
          disabled={s.pending} onclick={() => pickTip(t.a)}>
          <span class="te" aria-hidden="true">{t.e}</span>
          <b>{t.n}</b>
          <small>{plural(t.a)}</small>
        </button>
      {/each}
    </div>
    {#if s.err}<p class="err" id="tipErr">{s.err}</p>{/if}
    <div class="row">
      <button type="button" id="tipSend" disabled={!picked || s.pending} onclick={sendTip}>
        {s.pending ? 'Sending' : picked ? 'Send ' + picked.n + ' (' + plural(picked.a) + ')' : 'Pick a treat'}
      </button>
      <button type="button" class="ghost" id="tipCancel" onclick={closeTip}>Cancel</button>
    </div>
  </div>
{/if}
