/* AuthChrome.tsx — shared frame for auth screens (new design) */

import type { ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import { Icon } from './Icon';
import { LangSwitch } from './LangSwitch';
import { Logo } from './Logo';
import { ThemeSwitch } from './ThemeSwitch';

export function AuthChrome({ children }: { children: ReactNode }): JSX.Element {
  const navigate = useNavigate();
  return (
    <div className="fade-up" style={{ minHeight: '100vh', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <div style={{ position: 'absolute', inset: 0, backgroundImage: 'radial-gradient(var(--grid) 1px, transparent 1px)', backgroundSize: '26px 26px', maskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)', WebkitMaskImage: 'radial-gradient(ellipse 70% 50% at 50% 0%, #000, transparent 75%)', pointerEvents: 'none' }} />
      <div style={{ position: 'relative', display: 'flex', alignItems: 'center', padding: '20px clamp(20px,5vw,40px)' }}>
        <button
          type="button"
          onClick={() => navigate('/')}
          className="btn btn-quiet btn-sm row gap-2"
        >
          <Icon name="arrow-left" size={16} /> cotton
        </button>
        <span className="spacer" />
        <div className="row gap-2">
          <LangSwitch />
          <ThemeSwitch />
        </div>
      </div>
      <div style={{ position: 'relative', flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '24px 20px 64px' }}>
        <div style={{ width: 432, maxWidth: '100%' }}>{children}</div>
      </div>
    </div>
  );
}
