<script>
  import Avatar from './Avatar.svelte';
  import { chat, isMod, startReply, deleteOwn, modDelete, modUser, reportMessage, openThread, openUser, friendAct, hostUser, openTip, note } from '../lib/chat.svelte.js';
  import { ABBR, tierOf } from '../lib/rank.js';
  import { colorFor } from '../lib/avatar.js';
  import { tipFor } from '../lib/tips.js';

  let { m, tagged } = $props();

  const t = $derived(tierOf(m.r));
  const mine = $derived(!!chat.me && m.h.toLowerCase() === chat.me.toLowerCase());
  const roleCls = $derived(m.o ? ' r-' + m.o : '');
  const cls = $derived('m t' + t + roleCls + (mine ? ' me' : '') + (tagged ? ' mention' : ''));
  const ring = $derived(m.o === 'admin' ? '#e11d48' : m.o === 'mod' ? '#0d9488' : 'var(--t' + t + ')');
  const when = $derived(new Date(m.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }));

  const tip = $derived(m.ta ? tipFor(m.ta) : null);
  const toMe = $derived(!!m.tt && !!chat.me && m.tt.toLowerCase() === chat.me.toLowerCase());

  let menu = $state(''); // '', 'actions' or 'report'

  // Splits a message so @handle can be shown as a tag. Nothing here is ever inserted as HTML.
  const TAG_RE = /(^|[^A-Za-z0-9_])(@[A-Za-z0-9_]{3,16})(?![A-Za-z0-9_])/g;
  function parts(body) {
    const out = [];
    let last = 0, x;
    TAG_RE.lastIndex = 0;
    while ((x = TAG_RE.exec(body))) {
      const start = x.index + x[1].length;
      if (start > last) out.push({ text: body.slice(last, start) });
      out.push({ text: x[2], tag: true, tome: !!chat.me && x[2].slice(1).toLowerCase() === chat.me.toLowerCase() });
      last = start + x[2].length;
    }
    if (last < body.length) out.push({ text: body.slice(last) });
    return out;
  }

  function mention() {
    if (mine) return;
    const box = document.getElementById('text');
    if (!box) return;
    const tag = '@' + m.h + ' ';
    box.value = box.value && !/\s$/.test(box.value) ? box.value + ' ' + tag : box.value + tag;
    box.focus();
    box.setSelectionRange(box.value.length, box.value.length);
  }

  function jump(id) {
    const el = document.querySelector('.m[data-id="' + (id | 0) + '"]');
    if (!el) { note('sys', 'That message is no longer on your screen.'); return; }
    el.scrollIntoView({ block: 'center', behavior: 'smooth' });
    el.classList.add('flash');
    setTimeout(() => el.classList.remove('flash'), 1500);
  }

  const done = ok => { if (ok) menu = ''; };

  // Slide a message to the right to reply to it. The whole row moves, avatar included.
  function swipe(node, getMsg) {
    const row = node.parentElement;
    let x0 = 0, y0 = 0, dx = 0, active = false, locked = false;
    const reset = () => {
      row.style.transition = 'transform .2s';
      row.style.transform = '';
      row.style.setProperty('--sw', 0);
    };
    node.addEventListener('touchstart', e => {
      const p = e.touches[0];
      x0 = p.clientX; y0 = p.clientY; dx = 0; active = true; locked = false;
      row.style.transition = 'none';
    }, { passive: true });
    node.addEventListener('touchmove', e => {
      if (!active) return;
      const p = e.touches[0], mx = p.clientX - x0, my = p.clientY - y0;
      if (!locked) {
        if (Math.abs(mx) < 8 && Math.abs(my) < 8) return;
        if (Math.abs(my) > Math.abs(mx)) { active = false; return; } // the person is scrolling
        locked = true;
      }
      dx = Math.max(0, Math.min(mx, 90));
      row.style.transform = 'translateX(' + dx + 'px)';
      row.style.setProperty('--sw', Math.min(1, dx / 60));
    }, { passive: true });
    node.addEventListener('touchend', () => {
      if (!active) return;
      active = false;
      const go = locked && dx >= 60;
      reset();
      if (go) {
        if (navigator.vibrate) navigator.vibrate(10);
        startReply(getMsg());
      }
    });
    node.addEventListener('touchcancel', () => { active = false; reset(); });
  }
</script>

{#if m.ta}
  <div class="row tiprow">
    <div class={'m tipcard' + (toMe ? ' tome' : '') + (mine ? ' me' : '')} data-id={m.i}>
      <span class="te" aria-hidden="true">{tip ? tip.e : '\u{1F381}'}</span>
      <span class="tt"><b>@{m.h}</b> sent <b>@{m.tt}</b> a <b>{m.b}</b></span>
      <span class="tamt">{m.ta} credit{m.ta === 1 ? '' : 's'}</span>
      <time>{when}</time>
    </div>
  </div>
{:else}
<div class="row">
  <button type="button" class="avbtn" aria-label={'Open the profile of ' + m.h} onclick={() => openUser(m.h)}><Avatar handle={m.h} {ring} size={34} /></button>
  <div class={cls} data-id={m.i} use:swipe={() => m}>
    <div class="head">
      <span class={m.o ? 'bd r-' + m.o : 'bd t' + t}>{m.o ? m.o.toUpperCase() : ABBR[t]}</span>
      <button type="button" class={'who' + roleCls} style:--nc={colorFor(m.h)} title="Open profile" onclick={() => openUser(m.h)}>@{m.h}</button>
      <time>{when}</time>
      <button type="button" class="replybtn" aria-label={'Reply to @' + m.h} onclick={() => startReply(m)}>Reply</button>
      <button type="button" class="replybtn" aria-label={'More actions for the message from @' + m.h} onclick={() => (menu = menu ? '' : 'actions')}>&#8943;</button>
    </div>
    {#if m.q && m.qh}
      <div class="quote" style:--qc={colorFor(m.qh)} onclick={() => jump(m.q)} role="presentation">
        <b>@{m.qh}</b>
        <span>{m.qb || ''}</span>
      </div>
    {/if}
    <div>{#each parts(m.b) as p}{#if p.tag}<span class={'tag' + (p.tome ? ' tome' : '')}>{p.text}</span>{:else}{p.text}{/if}{/each}</div>
    {#if menu === 'actions'}
      <div class="actions">
        {#if !mine}<button type="button" class="ghost" onclick={() => done(mention())}>Mention</button>{/if}
        {#if !mine}<button type="button" class="ghost" onclick={() => done(openTip(m))}>Send a tip</button>{/if}
        {#if !mine}<button type="button" class="ghost" onclick={() => done(openThread(m.h))}>Message</button>{/if}
        {#if !mine}<button type="button" class="ghost" onclick={() => done(friendAct('add', m.h))}>Add friend</button>{/if}
        {#if !mine}<button type="button" class="ghost" onclick={() => (menu = 'report')}>Report</button>{/if}
        {#if isMod() && !mine}
          <button type="button" class="ghost" onclick={() => done(modUser('mute', m.h, 10))}>Mute 10 min</button>
          <button type="button" class="ghost" onclick={() => done(modUser('mute', m.h, 60))}>Mute 1 hour</button>
          <button type="button" class="warn" onclick={() => done(modUser('kick', m.h, 10))}>Kick 10 min</button>
        {/if}
        {#if chat.isHost && !isMod() && !mine && !m.o}
          <button type="button" class="ghost" onclick={() => done(hostUser('mute', m.h, 10))}>Mute 10 min here</button>
          <button type="button" class="warn" onclick={() => done(hostUser('kick', m.h, 10))}>Kick 10 min</button>
          <button type="button" class="warn" onclick={() => done(deleteOwn(m.i))}>Delete</button>
        {/if}
        {#if mine}
          <button type="button" class="warn" onclick={() => done(deleteOwn(m.i))}>Delete</button>
        {:else if isMod()}
          <button type="button" class="warn" onclick={() => done(modDelete(m.i))}>Delete</button>
        {/if}
      </div>
    {:else if menu === 'report'}
      <div class="actions">
        <span class="hint">Why?</span>
        <button type="button" class="ghost" onclick={() => done(reportMessage(m.i, 'spam'))}>Spam</button>
        <button type="button" class="ghost" onclick={() => done(reportMessage(m.i, 'abuse'))}>Abuse</button>
        <button type="button" class="ghost" onclick={() => done(reportMessage(m.i, 'other'))}>Other</button>
      </div>
    {/if}
  </div>
</div>
{/if}
