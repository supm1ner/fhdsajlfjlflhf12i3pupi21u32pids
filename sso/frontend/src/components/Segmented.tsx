/* Segmented.tsx — pill segmented control (from design tokens .seg) */

import { Icon, type IconName } from './Icon';

export interface SegmentedOption<V extends string> {
  value: V;
  label?: string;
  icon?: IconName;
}

interface SegmentedProps<V extends string> {
  options: ReadonlyArray<SegmentedOption<V>>;
  value: V;
  onChange: (value: V) => void;
  size?: 'sm' | 'md';
  ariaLabel?: string;
}

export function Segmented<V extends string>({
  options,
  value,
  onChange,
  size = 'md',
  ariaLabel,
}: SegmentedProps<V>): JSX.Element {
  return (
    <div className="seg" style={size === 'sm' ? { padding: 2 } : undefined}>
      {options.map((o) => (
        <button
          key={o.value}
          aria-pressed={value === o.value}
          onClick={() => onChange(o.value)}
          style={size === 'sm' ? { height: 28, padding: '0 11px', fontSize: 12.5 } : undefined}
        >
          {o.icon && <Icon name={o.icon} size={15} />}
          {o.label}
        </button>
      ))}
    </div>
  );
}
