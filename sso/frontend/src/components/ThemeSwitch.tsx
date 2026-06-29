/* ThemeSwitch.tsx — sun/moon toggle (new design) */

import { useApp } from '../i18n/context';
import { Icon } from './Icon';

export function ThemeSwitch(): JSX.Element {
  const { theme, setTheme } = useApp();
  return (
    <button
      type="button"
      title="Тема / Theme"
      aria-label={theme === 'dark' ? 'Switch to light theme' : 'Switch to dark theme'}
      onClick={() => setTheme(theme === 'dark' ? 'light' : 'dark')}
      className="btn btn-quiet btn-icon btn-sm"
    >
      <Icon name={theme === 'light' ? 'moon' : 'sun'} size={18} />
    </button>
  );
}
