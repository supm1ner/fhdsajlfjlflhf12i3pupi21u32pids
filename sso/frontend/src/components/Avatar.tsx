/* Avatar.tsx — circular avatar with deterministic color + initials (design ref) */

import type { CSSProperties } from 'react';

const AV_COLORS = ['#7c3aed', '#0ea5e9', '#10b981', '#f59e0b', '#ef4444', '#ec4899', '#6366f1', '#14b8a6'];

function hashIdx(s: string, n: number): number {
  let h = 0;
  for (let i = 0; i < (s || '').length; i++) h = (h * 31 + s.charCodeAt(i)) | 0;
  return Math.abs(h) % n;
}

function initials(name: string): string {
  const parts = (name || '?').trim().split(/\s+/);
  return ((parts[0]?.[0] ?? '') + (parts[1]?.[0] ?? '')).toUpperCase() || '?';
}

interface AvatarProps {
  name: string;
  src?: string | null;
  size?: number;
  ring?: boolean;
  style?: CSSProperties;
}

export function Avatar({ name, src, size = 44, ring, style }: AvatarProps): JSX.Element {
  const bg = AV_COLORS[hashIdx(name, AV_COLORS.length)];
  const isInit = initials(name);
  return (
    <div
      aria-hidden="true"
      className="avatar"
      style={{
        width: size,
        height: size,
        fontSize: size * 0.4,
        background: src ? 'transparent' : bg,
        boxShadow: ring ? '0 0 0 2px var(--bg), 0 0 0 4px var(--accent-tint-2)' : undefined,
        ...style,
      }}
    >
      {src ? (
        <img src={src} alt="" style={{ width: '100%', height: '100%', objectFit: 'cover' }} />
      ) : (
        isInit
      )}
    </div>
  );
}
