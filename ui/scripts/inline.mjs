// Puts the built script and stylesheet into one file for the Go server to hand out.
import { readFileSync, writeFileSync } from 'node:fs';

const css = readFileSync('dist/app.css', 'utf8');
const js = readFileSync('dist/app.js', 'utf8')
  .replace(/<\/script/gi, '<\\/script')
  .replace(/<!--/g, '<\\!--');
const shell = readFileSync('shell.html', 'utf8');
// A function as the replacement, so "$&" and similar in the code are never treated as patterns.
const html = shell.replace('/*STYLE*/', () => css).replace('/*SCRIPT*/', () => js);
writeFileSync('../web/index.html', html);
console.log('web/index.html written: ' + (html.length / 1024).toFixed(1) + ' KB');
