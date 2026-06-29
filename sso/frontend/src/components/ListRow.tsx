/* ListRow.tsx — icon + title/sub + trailing control row (new design) */

import type { CSSProperties, ReactNode } from 'react';
import { Icon, type IconName } from './Icon';

interface ListRowProps {
  icon?: IconName;
  title: ReactNode;
  sub?: ReactNode;
  right?: ReactNode;
  onClick?: () => void;
  style?: CSSProperties;
}

export function ListRow({ icon, title, sub, right, onClick, style }: ListRowProps): JSX.Element {
  return (
    <div
      onClick={onClick}
      className="row gap-3"
      style={{
        padding: '14px 0',
        borderBottom: '1px solid var(--border)',
        cursor: onClick ? 'pointer' : 'default',
        ...style,
      }}
    >
      {icon && (
        <div
          style={{
            width: 38,
            height: 38,
            flex: '0 0 auto',
            borderRadius: 10,
            background: 'var(--surface-2)',
            border: '1px solid var(--border)',
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: 'var(--text-2)',
          }}
        >
          <Icon name={icon} size={18} />
        </div>
      )}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ fontSize: 14.5, fontWeight: 540 }}>{title}</div>
        {sub && <div className="faint" style={{ fontSize: 13, marginTop: 2 }}>{sub}</div>}
      </div>
      {right}
    </div>
  );
}
