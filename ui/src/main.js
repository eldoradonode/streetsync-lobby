import './styles/tokens.css';
import './app.css';
import './styles/shell.css';
import './styles/components.css';
import './styles/chat.css';
import './styles/pages.css';
import './styles/staff.css';
import { mount, flushSync } from 'svelte';
import App from './App.svelte';
import { init } from './lib/chat.svelte.js';

window.__lobbyFlush = flushSync; // lets tests wait for the screen to catch up
init();
mount(App, { target: document.getElementById('root') });

// Keep the app exactly as tall as what the person can see. On phones the on-screen keyboard shrinks the visible
// area; sizing to it keeps the message box above the keyboard instead of hidden behind it.
const vv = window.visualViewport;
if (vv) {
  const fit = () => {
    if (vv.scale > 1.02) return; // pinch zoomed: leave the layout alone
    document.documentElement.style.setProperty('--app-h', Math.round(vv.height) + 'px');
    if (vv.offsetTop > 0) window.scrollTo(0, 0);
  };
  vv.addEventListener('resize', fit);
  vv.addEventListener('scroll', fit);
  fit();
}
