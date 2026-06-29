/* Button.tsx — variants: primary/accent/ghost/soft/quiet/danger */

import type { CSSProperties, ReactNode } from 'react';
import { Icon, type IconName } from './Icon';

type Variant = 'primary' | 'accent' | 'ghost' | 'soft' | 'quiet' | 'danger' | 'glass';
type Size = 'sm' | 'md' | 'lg';

interface ButtonProps {
  children?: ReactNode;
  variant?: Variant;
  size?: Size;
  icon?: IconName;
  iconRight?: IconName;
  full?: boolean;
  className?: string;
  onClick?: () => void;
  type?: 'button' | 'submit' | 'reset';
  disabled?: boolean;
  title?: string;
  style?: CSSProperties;
}

const SIZES: Record<Size, { padding: string; fontSize: number; height: number; borderRadius: string }> = {
  sm: { padding: '0 14px', fontSize: 13.5, height: 36, borderRadius: 'var(--r-sm)' },
  md: { padding: '0 20px', fontSize: 15, height: 44, borderRadius: 'var(--r-sm)' },
  lg: { padding: '0 26px', fontSize: 16, height: 52, borderRadius: 'var(--r-md)' },
};

const VARIANTS: Record<Variant, CSSProperties> = {
  primary: { background: 'var(--text)', color: 'var(--bg)', border: '1px solid transparent' },
  accent: { background: 'var(--accent)', color: 'var(--accent-fg)', border: '1px solid transparent' },
  ghost: { background: 'transparent', color: 'var(--text)', border: '1px solid var(--border-strong)' },
  soft: { background: 'var(--surface-2)', color: 'var(--text)', border: '1px solid var(--border)' },
  quiet: { background: 'transparent', color: 'var(--text-2)', padding: '0 12px', border: 'none' },
  glass: { background: 'color-mix(in srgb, var(--surface) 70%, transparent)', color: 'var(--text)', border: '1px solid var(--border)', backdropFilter: 'blur(8px)' },
  danger: { background: 'transparent', color: 'var(--bad)', border: '1px solid var(--border-strong)' },
};

export function Button({
  children,
  variant = 'primary',
  size = 'md',
  icon,
  iconRight,
  full,
  className,
  onClick,
  type = 'button',
  disabled = false,
  title,
  style,
}: ButtonProps): JSX.Element {
  const s = SIZES[size];
  const v = VARIANTS[variant];
  const base: CSSProperties = {
    display: full ? 'flex' : 'inline-flex',
    alignItems: 'center',
    justifyContent: 'center',
    gap: 9,
    height: s.height,
    padding: s.padding,
    fontSize: s.fontSize,
    fontWeight: 540,
    borderRadius: s.borderRadius,
    width: full ? '100%' : undefined,
    whiteSpace: 'nowrap',
    cursor: disabled ? 'not-allowed' : 'pointer',
    opacity: disabled ? 0.5 : 1,
    transition: 'background 0.18s var(--ease), border-color 0.18s var(--ease), color 0.18s var(--ease), transform 0.12s var(--ease)',
    lineHeight: 1,
    ...v,
    ...style,
  };

  return (
    <button
      type={type}
      onClick={disabled ? undefined : onClick}
      disabled={disabled}
      title={title}
      className={className}
      style={base}
    >
      {icon && <Icon name={icon} size={size === 'sm' ? 16 : 18} />}
      {children && <span>{children}</span>}
      {iconRight && <Icon name={iconRight} size={size === 'sm' ? 16 : 18} />}
    </button>
  );
}
