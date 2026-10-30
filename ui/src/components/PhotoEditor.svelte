<script>
  import { chat, loadPhotoStatus, uploadPhoto, removePhoto } from '../lib/chat.svelte.js';
  import { onMount } from 'svelte';

  const BOX = 260; // the square the person crops in, in CSS pixels
  const MAX_FILE = 15 * 1024 * 1024; // the page shrinks the picture, so the original can be large
  let fileEl;
  let img = $state(null); // { url, nw, nh }
  let zoom = $state(1);
  let ox = $state(0), oy = $state(0);
  let busy = $state(false);
  let err = $state('');
  let drag = null;

  const hasPhoto = $derived(!!chat.photos[(chat.me || '').toLowerCase()]);
  const base = $derived(img ? Math.max(BOX / img.nw, BOX / img.nh) : 1);
  const scale = $derived(base * zoom);

  onMount(() => { if (chat.photoOn === null) loadPhotoStatus(); });

  function clamp() {
    if (!img) return;
    const w = img.nw * scale, h = img.nh * scale;
    ox = Math.min(0, Math.max(BOX - w, ox));
    oy = Math.min(0, Math.max(BOX - h, oy));
  }
  function choose(e) {
    err = '';
    const f = e.target.files && e.target.files[0];
    e.target.value = '';
    if (!f) return;
    if (!/^image\/(jpeg|png|webp)$/.test(f.type)) { err = 'Choose a JPEG, PNG or WebP picture.'; return; }
    if (f.size > MAX_FILE) { err = 'That file is over 15 MB. Choose a smaller one.'; return; }
    const url = URL.createObjectURL(f);
    const im = new Image();
    im.onload = () => {
      if (img) URL.revokeObjectURL(img.url);
      if (im.naturalWidth < 48 || im.naturalHeight < 48) { err = 'That picture is too small. Use one at least 48 pixels wide and tall.'; URL.revokeObjectURL(url); return; }
      img = { url, nw: im.naturalWidth, nh: im.naturalHeight, el: im };
      zoom = 1;
      ox = (BOX - im.naturalWidth * Math.max(BOX / im.naturalWidth, BOX / im.naturalHeight)) / 2;
      oy = (BOX - im.naturalHeight * Math.max(BOX / im.naturalWidth, BOX / im.naturalHeight)) / 2;
    };
    im.onerror = () => { err = 'That file could not be read as a picture.'; URL.revokeObjectURL(url); };
    im.src = url;
  }
  function onZoom(e) {
    // keep the middle of the square where it is while zooming
    const cx = (BOX / 2 - ox) / scale, cy = (BOX / 2 - oy) / scale;
    zoom = +e.target.value;
    ox = BOX / 2 - cx * base * zoom;
    oy = BOX / 2 - cy * base * zoom;
    clamp();
  }
  function down(e) { drag = { x: e.clientX, y: e.clientY, ox, oy }; e.currentTarget.setPointerCapture(e.pointerId); }
  function move(e) { if (!drag) return; ox = drag.ox + e.clientX - drag.x; oy = drag.oy + e.clientY - drag.y; clamp(); }
  function up() { drag = null; }
  function cancel() { if (img) URL.revokeObjectURL(img.url); img = null; err = ''; }

  async function save() {
    if (!img || busy) return;
    busy = true; err = '';
    // The visible square, cut out of the original at full detail, shrunk to 512 px and saved as a JPEG.
    // This also turns WebP into JPEG and drops any hidden data in the original file.
    const side = BOX / scale, sx = -ox / scale, sy = -oy / scale;
    const c = document.createElement('canvas');
    c.width = c.height = 512;
    const g = c.getContext('2d');
    g.fillStyle = '#fff';
    g.fillRect(0, 0, 512, 512);
    g.drawImage(img.el, sx, sy, side, side, 0, 0, 512, 512);
    const blob = await new Promise(r => c.toBlob(r, 'image/jpeg', 0.85));
    if (!blob) { err = 'Could not prepare the picture. Try another one.'; busy = false; return; }
    const res = await uploadPhoto(blob);
    busy = false;
    if (res.ok) cancel(); else err = res.err;
  }
  async function remove() {
    if (busy) return;
    busy = true; err = '';
    const res = await removePhoto();
    busy = false;
    if (!res.ok) err = res.err;
  }
</script>

{#if chat.photoOn}
  <div class="photoed" id="photoEditor">
    {#if !img}
      <div class="row">
        <button type="button" class="ghost small" id="photoChoose" onclick={() => fileEl.click()}>{hasPhoto ? 'Change photo' : 'Add a photo'}</button>
        {#if hasPhoto}<button type="button" class="ghost small" id="photoRemove" disabled={busy} onclick={remove}>Remove photo</button>{/if}
      </div>
      <input bind:this={fileEl} type="file" accept="image/jpeg,image/png,image/webp" hidden onchange={choose} />
      <p class="hint">JPEG, PNG or WebP. You can crop it next. Your illustrated avatar comes back if you remove it.</p>
    {:else}
      <div class="cropbox" style:width={BOX + 'px'} style:height={BOX + 'px'} role="img" aria-label="Drag to move the picture"
        onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up}>
        <img src={img.url} alt="" draggable="false" style:width={img.nw * scale + 'px'} style:height={img.nh * scale + 'px'} style:transform={'translate(' + ox + 'px,' + oy + 'px)'} />
        <span class="cropring" aria-hidden="true"></span>
      </div>
      <label class="zoomrow">Zoom <input type="range" min="1" max="4" step="0.05" value={zoom} oninput={onZoom} aria-label="Zoom" /></label>
      <div class="row">
        <button type="button" id="photoSave" disabled={busy} onclick={save}>{busy ? 'Saving...' : 'Save photo'}</button>
        <button type="button" class="ghost" id="photoCancel" disabled={busy} onclick={cancel}>Cancel</button>
      </div>
    {/if}
    {#if err}<p class="warn-text" id="photoErr" role="alert">{err}</p>{/if}
  </div>
{/if}
