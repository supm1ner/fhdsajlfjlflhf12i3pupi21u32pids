import { mount } from 'svelte'
import './app.css'
import App from './App.svelte'
import { appState } from './lib/stores.svelte.js'
import { loginWithSavedToken, myUID } from './lib/tinode.js'

// Register the service worker for PWA install + offline app shell.
if ('serviceWorker' in navigator) {
  window.addEventListener('load', () => {
    navigator.serviceWorker.register('/sw.js').catch(() => { /* ignore */ });
  });
}

const app = mount(App, {
  target: document.getElementById('app'),
})

// Attempt to resume a previous session silently.
loginWithSavedToken()
  .then((ok) => {
    if (ok) {
      appState.user = { id: myUID(), name: 'You' };
      appState.connected = true;
      appState.view = 'app';
    }
  })
  .catch(() => { /* stay on login */ });

export default app
