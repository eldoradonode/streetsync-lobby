<script>
  // Simple line icons, drawn on a 24 by 24 grid. Nothing is loaded from outside.
  let { name, size = 22 } = $props();
  const PATHS = {
    home: ['M3 11.5 12 4l9 7.5', 'M5 10v10h5v-6h4v6h5V10'],
    chat: ['M21 12a8 8 0 0 1-11.7 7.1L4 20.5l1.4-4.9A8 8 0 1 1 21 12Z'],
    user: ['M4.5 20.5a7.5 7.5 0 0 1 15 0', 'M12 12.5a4.25 4.25 0 1 0 0-8.5 4.25 4.25 0 0 0 0 8.5Z'],
    rooms: ['M4 4h7v7H4z','M13 4h7v7h-7z','M4 13h7v7H4z','M13 13h7v7h-7z'],
    wallet: ['M3 7a2 2 0 0 1 2-2h12v3', 'M3 7v11a2 2 0 0 0 2 2h14a1 1 0 0 0 1-1V9a1 1 0 0 0-1-1H5a2 2 0 0 1-2-2Z', 'M16.5 14h.01'],
    shield: ['M12 3 5 6v5c0 4.5 3 8 7 10 4-2 7-5.5 7-10V6l-7-3Z'],
    flag: ['M5 21V4', 'M5 4h12l-2 4 2 4H5'],
    users: ['M16 20v-1.5a4 4 0 0 0-4-4H7a4 4 0 0 0-4 4V20', 'M9.5 11a3.5 3.5 0 1 0 0-7 3.5 3.5 0 0 0 0 7Z', 'M21 20v-1.5a4 4 0 0 0-3-3.9', 'M16 4.2a3.5 3.5 0 0 1 0 6.6'],
    list: ['M8 6h13', 'M8 12h13', 'M8 18h13', 'M3 6h.01', 'M3 12h.01', 'M3 18h.01'],
    chart: ['M4 20h16', 'M7 20v-7', 'M12 20V5', 'M17 20v-10'],
    back: ['M19 12H5', 'm12 19-7-7 7-7'],
    tools: ['M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76Z'],
    plus: ['M12 5v14', 'M5 12h14'],
    search: ['M11 18a7 7 0 1 0 0-14 7 7 0 0 0 0 14Z', 'm20 20-4-4'],
  };
</script>

<svg class="ic" width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
  {#each PATHS[name] || [] as d}<path {d} />{/each}
</svg>
