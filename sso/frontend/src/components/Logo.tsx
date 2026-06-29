/* Logo.tsx — cotton wordmark + 4-dot mark (from design/src/ui.jsx) */

import { CottonMark } from './Icon';

interface LogoProps {
  size?: number;
  onClick?: () => void;
  mark?: boolean;
  label?: boolean;
  accentMark?: boolean;
}

export function Logo({
  size = 30,
  onClick,
  mark = true,
  label = true,
  accentMark = false,
}: LogoProps): JSX.Element {
  const items: JSX.Element[] = [];
  if (mark) {
    items.push(
      <span key="m" style={{ display: 'flex', color: accentMark ? 'var(--accent)' : 'inherit' }}>
        <CottonMark size={size * 1.12} />
      </span>,
    );
  }
  if (label) {
    items.push(
      <span
        key="w"
        style={{
          fontSize: size * 0.82,
          fontWeight: 600,
          letterSpacing: '-0.03em',
          color: 'inherit',
          lineHeight: 1,
        }}
      >
        cotton
      </span>,
    );
  }
  return (
    <div
      className="row"
      style={{ gap: 9, cursor: onClick ? 'pointer' : 'default', color: 'var(--text)' }}
      onClick={onClick}
    >
      {items}
    </div>
  );
}
