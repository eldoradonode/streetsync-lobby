<script>
  // The admin console and the moderator dashboard. Two looks, one set of parts.
  // Hiding a button here is only a convenience: the server checks the role on every request.
  import { chat, setSection, go } from '../lib/chat.svelte.js';
  import { SECTIONS, AREA_NAME } from '../lib/areas.js';
  import Icon from './Icon.svelte';
  import StaffOverview from './StaffOverview.svelte';
  import StaffUsers from './StaffUsers.svelte';
  import StaffModerators from './StaffModerators.svelte';
  import StaffRooms from './StaffRooms.svelte';
  import StaffReports from './StaffReports.svelte';
  import StaffLog from './StaffLog.svelte';

  const area = $derived(chat.view === 'admin' || chat.view === 'mod' ? chat.view : '');
  const sec = $derived(area ? chat.section[area] : '');
</script>

<section id="staffArea" class={'area-' + area} hidden={!area}>
  {#if area}
    <div class="areabar" id="areaBar">
      <span class={'areabadge ' + area}><Icon name={area === 'admin' ? 'shield' : 'tools'} size={16} /> {area === 'admin' ? 'ADMIN' : 'MODERATOR'}</span>
      <span class="hint">{area === 'admin' ? 'Full control. Every action is written to the audit log.' : 'Moderation tools. Every action is written to the audit log.'}</span>
    </div>
    <div class="areatabs" role="tablist" aria-label={AREA_NAME[area] + ' sections'}>
      <button type="button" class="chipbtn" onclick={() => go('hub')}><Icon name="back" size={14} /> App</button>
      {#each SECTIONS[area] as s (s.id)}
        <button type="button" role="tab" aria-selected={sec === s.id} class={'chipbtn' + (sec === s.id ? ' on' : '')} onclick={() => setSection(area, s.id)}>{s.label}{#if s.id === 'reports' && chat.reportCount > 0}&nbsp;({chat.reportCount}){/if}</button>
      {/each}
    </div>

    {#if sec === 'overview'}<StaffOverview {area} />
    {:else if sec === 'users'}<StaffUsers {area} />
    {:else if sec === 'moderators' && area === 'admin'}<StaffModerators />
    {:else if sec === 'rooms'}<StaffRooms {area} />
    {:else if sec === 'reports'}<StaffReports />
    {:else if sec === 'log' && area === 'admin'}<StaffLog />
    {/if}
  {/if}
</section>
