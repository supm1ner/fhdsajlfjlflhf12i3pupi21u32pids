/* Panel.tsx — card surface (from design tokens .card) */

import type { CSSProperties, ReactNode } from 'react';

interface PanelProps {
  children: ReactNode;
  className?: string;
  pad?: number;
  style?: CSSProperties;
}

export function Panel({ children, className = '', pad = 26, style }: PanelProps): JSX.Element {
  return (
    <div
      className={'card' + (className ? ' ' + className : '')}
      style={{ padding: pad, ...style }}
    >
      {children}
    </div>
  );
}
