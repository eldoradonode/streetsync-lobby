<script>
  import Avatar from './Avatar.svelte';
  import { LOOKS, LOOK_COUNT, validLook } from '../lib/avatar.js';

  // value: the look chosen now, like "f3". onpick gets the new one.
  // Choosing a set (Male, Female or neither) only decides which eight pictures are shown. Nothing about it is stored.
  let { value = '', onpick } = $props();
  let set = $state(validLook(value) ? value[0] : 'x');

  const SETS = [['m', 'Male'], ['f', 'Female'], ['x', 'Prefer not to say']];
  function chooseSet(s) {
    set = s;
    if (!validLook(value) || value[0] !== s) onpick(s + '0');
  }
</script>

<div class="lookpicker">
  <div class="sets" role="radiogroup" aria-label="Pictures to choose from">
    {#each SETS as [k, label] (k)}
      <button type="button" role="radio" aria-checked={set === k} class={'ghost small' + (set === k ? ' sel' : '')} onclick={() => chooseSet(k)}>{label}</button>
    {/each}
  </div>
  <div class="looks" role="radiogroup" aria-label="Choose your avatar">
    {#each Array(LOOK_COUNT) as _, i}
      {@const code = set + i}
      <button type="button" role="radio" aria-checked={value === code} aria-label={'Avatar ' + (i + 1)} class={'lookbtn' + (value === code ? ' sel' : '')} data-look={code} onclick={() => onpick(code)}>
        <Avatar handle="pick" look={code} size={52} ring="transparent" />
      </button>
    {/each}
  </div>
</div>
