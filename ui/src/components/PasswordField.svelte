<script>
  import Icon from './Icon.svelte';
  // A password input with its own show/hide button. Each instance keeps its own state, so one eye
  // never changes another field. The button is a real button (keyboard and screen reader friendly)
  // with a 44px touch target, and it never submits the form.
  let { id, label, value = $bindable(''), autocomplete = 'off', maxlength = 128 } = $props();
  let shown = $state(false);
</script>

<label for={id}>{label}</label>
<div class="pwfield">
  <input {id} type={shown ? 'text' : 'password'} bind:value {maxlength} {autocomplete} autocapitalize="off" spellcheck="false">
  <button type="button" id={id + 'Eye'} class="pweye" aria-label={shown ? 'Hide ' + label.toLowerCase() : 'Show ' + label.toLowerCase()} aria-pressed={shown} aria-controls={id} onclick={() => (shown = !shown)}>
    <Icon name={shown ? 'eyeoff' : 'eye'} size={20} />
  </button>
</div>
