// Light/dark theme, matching the cotton (SSO) system. Persisted; defaults to the OS preference.

const KEY = 'cotton_theme';

let _theme = $state('light');

export function initTheme() {
  const saved = localStorage.getItem(KEY);
  const prefersDark = typeof window !== 'undefined' && window.matchMedia?.('(prefers-color-scheme: dark)').matches;
  _theme = saved || (prefersDark ? 'dark' : 'light');
  apply();
}

function apply() {
  if (typeof document !== 'undefined') document.documentElement.setAttribute('data-theme', _theme);
}

export function toggleTheme() {
  _theme = _theme === 'dark' ? 'light' : 'dark';
  localStorage.setItem(KEY, _theme);
  apply();
}

export const theme = {
  get value() { return _theme; },
  get isDark() { return _theme === 'dark'; },
};
