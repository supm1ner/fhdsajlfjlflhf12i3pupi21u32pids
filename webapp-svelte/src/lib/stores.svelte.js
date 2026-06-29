let _user = $state(null);
let _chats = $state([]);
let _currentChat = $state(null);
let _messages = $state([]);
let _view = $state('login');

export const appState = {
  get user() { return _user; },
  set user(v) { _user = v; },
  get chats() { return _chats; },
  set chats(v) { _chats = v; },
  get currentChat() { return _currentChat; },
  set currentChat(v) { _currentChat = v; },
  get messages() { return _messages; },
  set messages(v) { _messages = v; },
  get view() { return _view; },
  set view(v) { _view = v; },
};
