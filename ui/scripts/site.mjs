// Turns the build in dist/ (or dist-full/) into the files the Go server hands out.
//
//   node scripts/site.mjs         web/index.html, web/assets/*   (the real site)
//   node scripts/site.mjs full    tests/build/page.html          (one file, for the page tests)
import { readFileSync, writeFileSync, mkdirSync, rmSync, cpSync, readdirSync, statSync } from 'node:fs';
import { gzipSync } from 'node:zlib';

const full = process.argv[2] === 'full';
const shell = readFileSync('shell.html', 'utf8');
const kb = n => (n / 1024).toFixed(1) + ' KB';

if (full) {
  const css = readFileSync('dist-full/app.css', 'utf8');
  const js = readFileSync('dist-full/app.js', 'utf8').replace(/<\/script/gi, '<\\/script').replace(/<!--/g, '<\\!--');
  // A function as the replacement, so "$&" and similar in the code are never treated as patterns.
  const html = shell.replace('<style>/*STYLE*/</style>', () => '<style>' + css + '</style>').replace('<script>/*SCRIPT*/</script>', () => '<script>' + js + '</script>');
  mkdirSync('../tests/build', { recursive: true });
  writeFileSync('../tests/build/page.html', html);
  console.log('tests/build/page.html written: ' + kb(html.length));
} else {
  const manifest = JSON.parse(readFileSync('dist/.vite/manifest.json', 'utf8'));
  const entry = Object.values(manifest).find(e => e.isEntry);
  const links = (entry.css || []).map(f => `<link rel="stylesheet" href="/${f}">`).join('\n');
  const html = shell
    .replace('<style>/*STYLE*/</style>', () => links)
    .replace('<script>/*SCRIPT*/</script>', () => `<script type="module" crossorigin src="/${entry.file}"></script>`);
  rmSync('../web', { recursive: true, force: true });
  mkdirSync('../web/assets', { recursive: true });
  cpSync('dist/assets', '../web/assets', { recursive: true });
  writeFileSync('../web/index.html', html);

  // Report what a visitor downloads, measured from the files just written.
  const size = f => { const b = readFileSync(f); return [b.length, gzipSync(b, { level: 9 }).length]; };
  const rows = [['web/index.html', size('../web/index.html')]];
  for (const f of readdirSync('../web/assets').sort()) rows.push(['web/assets/' + f, size('../web/assets/' + f)]);
  console.log('file'.padEnd(44) + 'raw'.padStart(12) + 'gzip'.padStart(12));
  for (const [n, [r, g]] of rows) console.log(n.padEnd(44) + kb(r).padStart(12) + kb(g).padStart(12));
  const staffFiles = new Set([manifest['src/components/StaffArea.svelte']?.file, ...(manifest['src/components/StaffArea.svelte']?.css || [])]);
  let first = [0, 0], staff = [0, 0];
  for (const [n, [r, g]] of rows) {
    const isStaff = [...staffFiles].some(f => f && n.endsWith(f));
    (isStaff ? staff : first)[0] += r; (isStaff ? staff : first)[1] += g;
  }
  console.log('everyone downloads'.padEnd(44) + kb(first[0]).padStart(12) + kb(first[1]).padStart(12));
  console.log('moderators and admins also download'.padEnd(44) + kb(staff[0]).padStart(12) + kb(staff[1]).padStart(12));
}
