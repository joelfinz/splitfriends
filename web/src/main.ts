import { mount } from 'svelte';
import './app.css';
import App from './App.svelte';
import { applyTheme, getThemePref } from './lib/theme';
import { navigate } from './lib/router.svelte';

applyTheme(getThemePref());

const app = mount(App, { target: document.getElementById('app')! });

if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch((e) => console.warn('sw register failed', e));
  });
  // Fallback used by sw.js when WindowClient.navigate() is unavailable.
  navigator.serviceWorker.addEventListener('message', (e) => {
    if (e.data?.type === 'navigate' && typeof e.data.url === 'string') {
      const u = new URL(e.data.url, location.origin);
      navigate(u.pathname + u.search);
    }
  });
}

export default app;
