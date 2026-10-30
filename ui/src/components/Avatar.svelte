<script>
  import { avatarOf } from '../lib/avatar.js';
  import { chat } from '../lib/chat.svelte.js';
  // look: show this exact look (the picker uses it). Otherwise the look everyone sees for this handle.
  let { handle, ring = '#8a8a9a', size = 36, look = '' } = $props();
  const a = $derived(avatarOf(handle || '?', look || chat.avatars[(handle || '').toLowerCase()]));
  const ink = '#1b1b24';
  // A profile photo, when the person has one. The small picture is enough up to 56 px; the profile page asks for the larger one.
  // If it cannot load, the illustrated avatar below stays in place.
  const ver = $derived(chat.photos[(handle || '').toLowerCase()] || '');
  const src = $derived(ver && !look ? '/photo/' + encodeURIComponent(handle) + '?v=' + ver + (size > 56 ? '&s=m' : '') : '');
  let failed = $state('');
  const showPhoto = $derived(!!src && failed !== src);
</script>

<span class="av" style:width={size + 'px'} style:height={size + 'px'} style:--ring={ring}>
  {#if showPhoto}
    <img class="avphoto" {src} width={size} height={size} alt={'Photo of ' + handle} loading="lazy" decoding="async" onerror={() => (failed = src)} />
  {/if}
  {#if !showPhoto}
  <svg viewBox="0 0 40 40" width={size} height={size} role="img" aria-label={'Avatar of ' + handle}>
    <rect width="40" height="40" fill={a.bg} />
    {#if a.hairStyle === 5}<path d="M10.5 19 C9.5 8 15 6.5 20 6.5 C25 6.5 30.5 8 29.5 19 L31 31 L25.5 31 L25.5 21 L14.5 21 L14.5 31 L9 31Z" fill={a.hair} />{/if}
    <path d="M3 40 Q6 29 20 29 Q34 29 37 40Z" fill={a.shirt} />
    <rect x="17" y="25" width="6" height="6" rx="2" fill={a.skin} />
    <ellipse cx="20" cy="19" rx="8.5" ry="9.5" fill={a.skin} />
    {#if a.hairStyle === 0 || a.hairStyle === 2 || a.hairStyle === 5}
      <path d="M11.5 17 C11 9 16 8 20 8 C24 8 29 9 28.5 17 C26 13.5 22.5 12.5 20 12.5 C17.5 12.5 14 13.5 11.5 17Z" fill={a.hair} />
    {/if}
    {#if a.hairStyle === 2}<circle cx="20" cy="6.5" r="4" fill={a.hair} />{/if}
    {#if a.hairStyle === 1}
      <circle cx="13.5" cy="13" r="4" fill={a.hair} /><circle cx="20" cy="10.5" r="4.6" fill={a.hair} />
      <circle cx="26.5" cy="13" r="4" fill={a.hair} /><circle cx="11.5" cy="18" r="3" fill={a.hair} /><circle cx="28.5" cy="18" r="3" fill={a.hair} />
    {/if}
    {#if a.hairStyle === 3}
      <path d="M11 15 C11 8 16 7 20 7 C24 7 29 8 29 15Z" fill={a.shirt} /><rect x="9.5" y="14" width="21" height="3" rx="1.5" fill={a.shirt} />
    {/if}
    {#if a.eyes === 0}
      <circle cx="16.6" cy="20" r="1.2" fill={ink} /><circle cx="23.4" cy="20" r="1.2" fill={ink} />
    {:else if a.eyes === 1}
      <circle cx="16.6" cy="20" r="2.4" fill="none" stroke={ink} stroke-width="1" /><circle cx="23.4" cy="20" r="2.4" fill="none" stroke={ink} stroke-width="1" />
      <path d="M19 20 L21 20" stroke={ink} stroke-width="1" /><circle cx="16.6" cy="20" r="0.9" fill={ink} /><circle cx="23.4" cy="20" r="0.9" fill={ink} />
    {:else}
      <path d="M14.8 20.5 Q16.6 18.2 18.4 20.5" fill="none" stroke={ink} stroke-width="1.2" stroke-linecap="round" />
      <path d="M21.6 20.5 Q23.4 18.2 25.2 20.5" fill="none" stroke={ink} stroke-width="1.2" stroke-linecap="round" />
    {/if}
    {#if a.mouth === 0}
      <path d="M17 24.5 Q20 27 23 24.5" fill="none" stroke={ink} stroke-width="1.2" stroke-linecap="round" />
    {:else if a.mouth === 1}
      <path d="M16.5 24 Q20 28.5 23.5 24Z" fill="#7a2a2a" />
    {:else}
      <path d="M17.5 25.2 L22.5 25.2" stroke={ink} stroke-width="1.2" stroke-linecap="round" />
    {/if}
  </svg>
  {/if}
</span>
