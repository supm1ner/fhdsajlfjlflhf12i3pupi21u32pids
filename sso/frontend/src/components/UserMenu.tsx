/* UserMenu.tsx — signed-in header affordance (new design) */

import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { useApp } from '../i18n/context';
import { api, type Account, type User } from '../lib/api';
import { Avatar } from './Avatar';
import { Icon } from './Icon';

type MenuUser = Pick<User, 'displayName' | 'username'> & { avatarUrl?: string };

export function UserMenu({ user }: { user: MenuUser | Account }): JSX.Element {
  const { t } = useApp();
  const navigate = useNavigate();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);

  const name = user.displayName || user.username;
  const firstName = name.split(' ')[0] ?? name;

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent): void => {
      if (rootRef.current && !rootRef.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent): void => {
      if (e.key === 'Escape') setOpen(false);
    };
    document.addEventListener('mousedown', onDown);
    document.addEventListener('keydown', onKey);
    return () => {
      document.removeEventListener('mousedown', onDown);
      document.removeEventListener('keydown', onKey);
    };
  }, [open]);

  async function onLogout(): Promise<void> {
    setOpen(false);
    try {
      await api.logout();
    } catch { /* ignore */ }
    navigate('/');
  }

  return (
    <div ref={rootRef} style={{ position: 'relative' }}>
      <button
        type="button"
        aria-haspopup="menu"
        aria-expanded={open}
        aria-label={t.acc_menu_account}
        onClick={() => setOpen((v) => !v)}
        className="row gap-2"
        style={{
          padding: '3px 6px 3px 12px',
          borderRadius: 'var(--r-pill)',
          background: 'var(--surface)',
          border: '1px solid var(--border-strong)',
          color: 'var(--text)',
          fontWeight: 540,
          fontSize: 14,
          cursor: 'pointer',
        }}
      >
        <span className="faint" style={{ fontSize: 13.5 }}>{firstName}</span>
        <Avatar name={name} src={user.avatarUrl} size={32} />
      </button>

      {open && (
        <div
          role="menu"
          className="card scale-in"
          style={{
            position: 'absolute',
            top: 'calc(100% + 6px)',
            right: 0,
            minWidth: 190,
            padding: 6,
            borderRadius: 'var(--r-md)',
            zIndex: 60,
            boxShadow: 'var(--shadow-lg)',
          }}
        >
          <button
            type="button"
            role="menuitem"
            onClick={() => { setOpen(false); navigate('/account'); }}
            className="row gap-3"
            style={{
              width: '100%', padding: '9px 12px', borderRadius: 8,
              border: 'none', background: 'transparent', cursor: 'pointer',
              color: 'var(--text)', fontSize: 14, fontWeight: 500, textAlign: 'left',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-hover)'; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
          >
            <Icon name="user" size={17} style={{ color: 'var(--text-3)' }} />
            {t.acc_menu_account}
          </button>
          <button
            type="button"
            role="menuitem"
            onClick={onLogout}
            className="row gap-3"
            style={{
              width: '100%', padding: '9px 12px', borderRadius: 8,
              border: 'none', background: 'transparent', cursor: 'pointer',
              color: 'var(--bad)', fontSize: 14, fontWeight: 500, textAlign: 'left',
            }}
            onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--bad-tint)'; }}
            onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}
          >
            <Icon name="logout" size={17} />
            {t.acc_menu_logout}
          </button>
        </div>
      )}
    </div>
  );
}
