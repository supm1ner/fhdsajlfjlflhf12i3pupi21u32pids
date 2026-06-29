/* Badge.tsx — pill badge with optional dot and tone (from design tokens) */

import type { CSSProperties, ReactNode } from 'react';

type Tone = 'good' | 'warn' | 'bad' | 'accent' | undefined;

interface BadgeProps {
  children: ReactNode;
  dot?: boolean;
  icon?: IconName;
  tone?: Tone;
  color?: string;
  bg?: string;
  style?: CSSProperties;
}

import { Icon, type IconName } from './Icon';

export function Badge({ children, dot, icon, tone, color, bg, style }: BadgeProps): JSX.Element {
  const cls = 'badge' + (tone ? ' badge-' + tone : '');
  return (
    <span className={cls} style={{ color, background: bg, ...style }}>
      {dot && <span className="dot" />}
      {icon && <Icon name={icon} size={13} />}
      {children}
    </span>
  );
}
