/* ============================================================================
 * PasswordStrength.tsx — three-bar strength meter for the signup screen
 * (from _design_ref/screen-auth.jsx). Advisory only; backend enforces policy.
 * ==========================================================================*/

import type { Dict } from '../i18n/dictionary';
import { strength, STRENGTH_COLORS } from '../lib/strength';

export function PasswordStrength({ password, t }: { password: string; t: Dict }): JSX.Element | null {
  if (!password) return null;
  const s = strength(password);
  // Label per score index (prototype: weak, weak, ok, strong).
  const labels = [t.pw_weak, t.pw_weak, t.pw_ok, t.pw_strong];
  const color = STRENGTH_COLORS[s];

  return (
    <div className="col" style={{ gap: 7 }} data-testid="password-strength">
      <div className="row" style={{ gap: 6 }}>
        {[0, 1, 2].map((i) => (
          <div
            key={i}
            style={{
              flex: 1,
              height: 5,
              borderRadius: 99,
              background: i < s ? color : 'var(--glass-3)',
              transition: 'all .4s var(--ease)',
            }}
          />
        ))}
      </div>
      <span style={{ fontSize: 12.5, color, fontWeight: 600, paddingLeft: 2 }}>{labels[s]}</span>
    </div>
  );
}
