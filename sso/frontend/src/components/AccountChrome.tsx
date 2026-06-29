/* AccountChrome.tsx — top nav frame for signed-in screens (new design) */

import type { ReactNode } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Account } from '../lib/api';
import { LangSwitch } from './LangSwitch';
import { Logo } from './Logo';
import { ThemeSwitch } from './ThemeSwitch';
import { UserMenu } from './UserMenu';

export function AccountChrome({
  user,
  children,
}: {
  user: Account;
  children: ReactNode;
}): JSX.Element {
  const navigate = useNavigate();
  return (
    <div>
      <header
        style={{
          position: 'sticky', top: 0, zIndex: 40,
          display: 'flex', alignItems: 'center', gap: 14,
          padding: '14px clamp(16px,4vw,32px)',
          borderBottom: '1px solid var(--border)',
          background: 'color-mix(in srgb, var(--bg) 85%, transparent)',
          backdropFilter: 'blur(12px)',
        }}
      >
        <Logo size={19} accentMark onClick={() => navigate('/')} />
        <span className="spacer" />
        <LangSwitch />
        <ThemeSwitch />
        <div className="vdivider" style={{ height: 26, margin: '0 2px' }} />
        <UserMenu user={user} />
      </header>
      {children}
    </div>
  );
}
