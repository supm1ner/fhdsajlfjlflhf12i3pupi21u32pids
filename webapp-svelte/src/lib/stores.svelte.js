// Global reactive application state (Svelte 5 runes).

let _user = $state(null);          // { id, name, avatar }
let _contacts = $state([]);        // array of contact descriptors (see messaging.js)
let _currentTopic = $state(null);  // topic name of the open conversation
let _view = $state('login');       // 'login' | 'register' | 'app' | 'settings'
let _connected = $state(false);
let _call = $state(null);          // active call descriptor (see calls.js) or null

export const appState = {
  get user() { return _user; },
  set user(v) { _user = v; },
  get contacts() { return _contacts; },
  set contacts(v) { _contacts = v; },
  get currentTopic() { return _currentTopic; },
  set currentTopic(v) { _currentTopic = v; },
  get view() { return _view; },
  set view(v) { _view = v; },
  get connected() { return _connected; },
  set connected(v) { _connected = v; },
  get call() { return _call; },
  set call(v) { _call = v; },
};
