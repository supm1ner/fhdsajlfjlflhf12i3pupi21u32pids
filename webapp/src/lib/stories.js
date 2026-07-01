// Stories store.
//
// Cross-user story distribution requires a backend broadcast channel (see
// docs/STORIES_AND_BOTS.md). Until that exists, a user's own stories are kept in
// localStorage so the feature is fully usable on-device: post, view, auto-expire.
// The API below is the single seam a backend implementation would replace.

const STORY_TTL_MS = 24 * 60 * 60 * 1000; // stories live 24h
const KEY_PREFIX = 'sunrise.stories.';

function key(uid) {
  return KEY_PREFIX + (uid || 'me');
}

// Returns non-expired stories for a user, oldest first: [{id, img, ts}].
export function loadStories(uid) {
  let list = [];
  try {
    list = JSON.parse(localStorage.getItem(key(uid)) || '[]');
  } catch (e) {
    list = [];
  }
  const cutoff = Date.now() - STORY_TTL_MS;
  const fresh = list.filter(s => s && s.ts > cutoff);
  if (fresh.length != list.length) {
    save(uid, fresh);
  }
  return fresh;
}

function save(uid, list) {
  try {
    localStorage.setItem(key(uid), JSON.stringify(list));
  } catch (e) {
    // Quota exceeded or storage disabled — stories just won't persist.
  }
}

// Appends a story (base64 data URL) and returns the updated list.
export function addStory(uid, img) {
  const list = loadStories(uid);
  list.push({id: '' + Date.now() + Math.round(Math.random() * 1e6), img, ts: Date.now()});
  save(uid, list);
  return list;
}

export function hasStories(uid) {
  return loadStories(uid).length > 0;
}

export function storyTtlMs() {
  return STORY_TTL_MS;
}
