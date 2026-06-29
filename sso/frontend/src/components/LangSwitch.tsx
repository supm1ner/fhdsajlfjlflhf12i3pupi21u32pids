/* LangSwitch.tsx — RU/EN pill (new design) */

import { useApp } from '../i18n/context';
import { Icon } from './Icon';

export function LangSwitch(): JSX.Element {
  const { lang, setLang } = useApp();
  return (
    <div className="seg">
      {(['ru', 'en'] as const).map((l) => (
        <button
          key={l}
          aria-pressed={lang === l}
          onClick={() => setLang(l)}
          style={{ textTransform: 'uppercase', letterSpacing: '0.04em' }}
        >
          {l}
        </button>
      ))}
    </div>
  );
}
