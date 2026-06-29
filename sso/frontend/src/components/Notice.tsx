/* Notice.tsx — small inline banner for form errors / success messages */

import type { ReactNode } from 'react';

type Tone = 'error' | 'success' | 'info';

const TONES: Record<Tone, { color: string; bg: string; border: string }> = {
  error: { color: 'var(--bad)', bg: 'var(--bad-tint)', border: 'var(--bad)' },
  success: { color: 'var(--good)', bg: 'var(--good-tint)', border: 'transparent' },
  info: { color: 'var(--text-2)', bg: 'var(--surface-2)', border: 'var(--border)' },
};

export function Notice({ tone = 'info', children }: { tone?: Tone; children: ReactNode }): JSX.Element {
  const c = TONES[tone];
  return (
    <div
      role={tone === 'error' ? 'alert' : 'status'}
      style={{
        padding: '11px 14px',
        borderRadius: 'var(--r-sm)',
        background: c.bg,
        border: '1px solid ' + c.border,
        color: c.color,
        fontSize: 14,
        fontWeight: 500,
        lineHeight: 1.45,
      }}
    >
      {children}
    </div>
  );
}
