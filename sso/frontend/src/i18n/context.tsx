/* ============================================================================
 * context.tsx — theme + language context with localStorage persistence
 *
 * Mirrors the prototype's App-level state (app.jsx) but persists choices to
 * localStorage (build contract §7 / spec "Theme toggle persists",
 * "Locale toggle switches all copy"). Defaults: RU + dark (design D9 / app.jsx
 * TWEAK_DEFAULTS). The provider writes `data-theme`, `lang` and the `--h` accent
 * hue onto <html> exactly as the prototype did.
 * ==========================================================================*/

import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react';
import { I18N, type Dict, type Lang } from './dictionary';

export type Theme = 'dark' | 'light';

type TFunc = Dict & { (a: string, b?: string): string };

interface AppContextValue {
  t: TFunc;
  lang: Lang;
  theme: Theme;
  setLang: (lang: Lang) => void;
  setTheme: (theme: Theme) => void;
}

const THEME_KEY = 'cid_theme';
const LANG_KEY = 'cid_lang';

/** Prototype default accent hue (app.jsx TWEAK_DEFAULTS / tokens `--h`). */
const DEFAULT_HUE = 268;

const AppContext = createContext<AppContextValue | null>(null);

function readStored<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  if (typeof window === 'undefined') return fallback;
  const stored = window.localStorage.getItem(key);
  return stored && (allowed as readonly string[]).includes(stored) ? (stored as T) : fallback;
}

export function AppProvider({ children }: { children: ReactNode }): JSX.Element {
  // RU + dark defaults, restored from localStorage when present.
  const [theme, setThemeState] = useState<Theme>(() =>
    readStored<Theme>(THEME_KEY, ['dark', 'light'], 'dark'),
  );
  const [lang, setLangState] = useState<Lang>(() => readStored<Lang>(LANG_KEY, ['ru', 'en'], 'ru'));

  // Apply theme + lang to <html> so the CSS token blocks switch (tokens.css).
  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme);
  }, [theme]);
  useEffect(() => {
    document.documentElement.setAttribute('lang', lang);
  }, [lang]);
  // Pin the default accent hue (the prototype's tweaks panel is dev-only and not
  // shipped; the hue stays at the design default).
  useEffect(() => {
    document.documentElement.style.setProperty('--h', String(DEFAULT_HUE));
  }, []);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    try {
      window.localStorage.setItem(THEME_KEY, next);
    } catch {
      /* localStorage may be unavailable (private mode); ignore — non-fatal. */
    }
  }, []);

  const setLang = useCallback((next: Lang) => {
    setLangState(next);
    try {
      window.localStorage.setItem(LANG_KEY, next);
    } catch {
      /* ignore */
    }
  }, []);

  // t is a callable function + property proxy: t(key) for dict lookup, t(ru,en) for lang fallback,
  // t.key for direct property access.
  const t = useMemo(() => {
    const dict = I18N[lang];
    const fn = (a: string, b?: string) => (b === undefined ? dict[a] ?? a : (lang === 'ru' ? a : b));
    return new Proxy(fn, {
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      get: (_target: any, prop: string) => (dict as any)[prop] ?? prop,
    }) as unknown as Dict & ((a: string, b?: string) => string);
  }, [lang]);

  const value = useMemo<AppContextValue>(
    () => ({ t, lang, theme, setLang, setTheme }),
    [t, lang, theme, setLang, setTheme],
  );

  return <AppContext.Provider value={value}>{children}</AppContext.Provider>;
}

// Provider + consumer hook colocated by design; fast-refresh's "components only"
// rule doesn't apply to this context module.
// eslint-disable-next-line react-refresh/only-export-components
export function useApp(): AppContextValue {
  const ctx = useContext(AppContext);
  if (!ctx) {
    throw new Error('useApp must be used within <AppProvider>');
  }
  return ctx;
}
