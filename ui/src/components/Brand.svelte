<script>
  let { size = 28, word = true } = $props();
</script>

<span class="brand">
  <svg width={size} height={size} viewBox="0 0 32 32" aria-hidden="true">
    <rect width="32" height="32" rx="9" fill="#0D1420" stroke="#1D2A3E" />
    <path d="M10 21c0-2.2 1.8-3 6-3.6 3.4-.5 4.2-1.6 4.2-2.9 0-1.5-1.5-2.5-4-2.5-2.3 0-3.9.9-4.7 2.6" fill="none" stroke="#00D9E8" stroke-width="2.4" stroke-linecap="round" />
    <path d="M22 11c0 2.2-1.8 3-6 3.6-3.4.5-4.2 1.6-4.2 2.9 0 1.5 1.5 2.5 4 2.5 2.3 0 3.9-.9 4.7-2.6" fill="none" stroke="#8B5CF6" stroke-width="2.4" stroke-linecap="round" opacity="0" />
  </svg>
  {#if word}<b>STREETSYNC</b>{/if}
</span>
