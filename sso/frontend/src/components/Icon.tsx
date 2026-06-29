/* cotton — line icon set. Icon({name, size, stroke}) */

import type { CSSProperties } from 'react';

const P: Record<string, string> = {
  'arrow-right': 'M5 12h14M13 6l6 6-6 6',
  'arrow-left': 'M19 12H5M11 6l-6 6 6 6',
  'chevron-right': 'M9 6l6 6-6 6',
  'chevron-down': 'M6 9l6 6 6-6',
  'chevron-up': 'M6 15l6-6 6 6',
  'chevron-left': 'M15 6l-6 6 6 6',
  check: 'M5 12.5l4.5 4.5L19 7',
  x: 'M6 6l12 12M18 6L6 18',
  plus: 'M12 5v14M5 12h14',
  minus: 'M5 12h14',
  mail: 'M3.5 6.5h17v11h-17zM4 7l8 6 8-6',
  lock: 'M6.5 10.5V8a5.5 5.5 0 0111 0v2.5M5 10.5h14v9H5zM12 14.5v2.5',
  key: 'M15.5 8.5a3.5 3.5 0 11-3.4 4.4L7 18l-2 0 0-2 .9-.9M5 16l2.2-2.2M9 13l2-2',
  fingerprint: 'M12 4.5a7.5 7.5 0 00-7.5 7.5M19.5 12a7.5 7.5 0 00-3.6-6.4M7 12a5 5 0 0110 0v3M12 12v4.5M9.2 15.5v1.2M14.8 14v3',
  eye: 'M2.5 12S6 5.5 12 5.5 21.5 12 21.5 12 18 18.5 12 18.5 2.5 12 2.5 12zM12 15a3 3 0 100-6 3 3 0 000 6z',
  'eye-off': 'M3 3l18 18M10.6 10.7a3 3 0 004.2 4.2M9.4 5.9A9.7 9.7 0 0112 5.5c6 0 9.5 6.5 9.5 6.5a16 16 0 01-2.8 3.6M6.2 7.3A15.7 15.7 0 002.5 12s3.5 6.5 9.5 6.5a9.4 9.4 0 003.4-.6',
  shield: 'M12 3.5l7 2.6v5.3c0 4.3-3 7.4-7 9.1-4-1.7-7-4.8-7-9.1V6.1z',
  'shield-check': 'M12 3.5l7 2.6v5.3c0 4.3-3 7.4-7 9.1-4-1.7-7-4.8-7-9.1V6.1zM8.8 12l2.2 2.2L15.4 10',
  user: 'M12 12a4 4 0 100-8 4 4 0 000 8zM5 20c.8-3.4 3.6-5 7-5s6.2 1.6 7 5',
  users: 'M9 11a3.5 3.5 0 100-7 3.5 3.5 0 000 7zM3 19c.7-3 2.9-4.5 6-4.5s5.3 1.5 6 4.5M16 4.2a3.5 3.5 0 010 6.6M18 14.4c2.2.5 3.4 1.9 4 4.6',
  grid: 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h6v6h-6z',
  apps: 'M5 5h4v4H5zM15 5h4v4h-4zM5 15h4v4H5zM15 15h4v4h-4z',
  monitor: 'M3.5 5h17v11h-17zM9 20h6M12 16v4',
  smartphone: 'M8 3.5h8v17H8zM11 17.5h2',
  laptop: 'M6 6h12v9H6zM3 18h18l-1.5-3M9 6.5',
  tablet: 'M6 3.5h12v17H6zM11 17h2',
  globe: 'M12 21a9 9 0 100-18 9 9 0 000 18zM3.5 12h17M12 3c2.5 2.4 3.8 5.6 3.8 9s-1.3 6.6-3.8 9c-2.5-2.4-3.8-5.6-3.8-9S9.5 5.4 12 3z',
  sun: 'M12 7.5a4.5 4.5 0 100 9 4.5 4.5 0 000-9zM12 2v2.5M12 19.5V22M4.2 4.2l1.8 1.8M18 18l1.8 1.8M2 12h2.5M19.5 12H22M4.2 19.8l1.8-1.8M18 6l1.8-1.8',
  moon: 'M20 14.5A8 8 0 119.5 4a6.5 6.5 0 1010.5 10.5z',
  settings: 'M12 15a3 3 0 100-6 3 3 0 000 6zM19.4 13.5a1.6 1.6 0 00.3 1.8l.1.1a2 2 0 11-2.8 2.8l-.1-.1a1.6 1.6 0 00-2.7 1.1v.2a2 2 0 11-4 0v-.1a1.6 1.6 0 00-1-1.5 1.6 1.6 0 00-1.8.3l-.1.1a2 2 0 11-2.8-2.8l.1-.1a1.6 1.6 0 00-1.1-2.7H3.5a2 2 0 110-4h.1a1.6 1.6 0 001.5-1 1.6 1.6 0 00-.3-1.8l-.1-.1a2 2 0 112.8-2.8l.1.1a1.6 1.6 0 001.8.3h.1a1.6 1.6 0 001-1.5V3.5a2 2 0 114 0v.1a1.6 1.6 0 001 1.5 1.6 1.6 0 001.8-.3l.1-.1a2 2 0 112.8 2.8l-.1.1a1.6 1.6 0 00-.3 1.8v.1a1.6 1.6 0 001.5 1h.2a2 2 0 110 4h-.1a1.6 1.6 0 00-1.5 1z',
  bell: 'M18 9a6 6 0 10-12 0c0 6-2.5 7.5-2.5 7.5h17S18 15 18 9zM10 20a2 2 0 004 0',
  logout: 'M9 20H5V4h4M15.5 16l4-4-4-4M19.5 12H9',
  trash: 'M4 7h16M9.5 7V5h5v2M6 7l1 13h10l1-13M10 11v6M14 11v6',
  edit: 'M4 20l4-1 11-11-3-3L5 16l-1 4zM14 6l3 3',
  copy: 'M9 9h11v11H9zM5 15H4V4h11v1',
  'external-link': 'M14 4h6v6M20 4l-9 9M18 13v6H5V6h6',
  search: 'M11 18a7 7 0 100-14 7 7 0 000 14zM20 20l-4-4',
  'more-h': 'M5 12a1 1 0 102 0 1 1 0 00-2 0zM11 12a1 1 0 102 0 1 1 0 00-2 0zM17 12a1 1 0 102 0 1 1 0 00-2 0z',
  clock: 'M12 21a9 9 0 100-18 9 9 0 000 18zM12 7.5V12l3 2',
  activity: 'M3 12h4l2.5-7 5 14L17 12h4',
  chart: 'M4 20V4M4 20h16M8 16v-4M12 16V9M16 16v-7',
  'pie-chart': 'M12 3v9h9a9 9 0 11-9-9zM21 12a9 9 0 00-9-9',
  'alert-triangle': 'M12 8v5M12 16.5v.5M10.3 4.2L3 17a2 2 0 001.7 3h14.6a2 2 0 001.7-3L13.7 4.2a2 2 0 00-3.4 0z',
  info: 'M12 21a9 9 0 100-18 9 9 0 000 18zM12 11v5M12 8v.5',
  'map-pin': 'M12 21s7-5.5 7-11a7 7 0 10-14 0c0 5.5 7 11 7 11zM12 12.5a2.5 2.5 0 100-5 2.5 2.5 0 000 5z',
  'qr-code': 'M4 4h6v6H4zM14 4h6v6h-6zM4 14h6v6H4zM14 14h2v2h-2zM18 14h2v2h-2zM14 18h2v2h-2zM18 18h2v2h-2z',
  refresh: 'M20 11a8 8 0 10-1 5M20 6v5h-5',
  menu: 'M4 7h16M4 12h16M4 17h16',
  filter: 'M4 6h16l-6 7v5l-4 2v-7z',
  download: 'M12 4v11M7.5 11l4.5 4 4.5-4M5 19h14',
  building: 'M5 21V5l8-2v18M13 21V9l6 2v10M9 8h.01M9 12h.01M9 16h.01M16 13h.01M16 17h.01M3 21h18',
  'mail-app': 'M3 7l9 6 9-6M3 6h18v12H3z',
  disk: 'M5 16a4 4 0 011-7.9A5.5 5.5 0 0117 8a4.5 4.5 0 011 8.9z',
  music: 'M9 18V6l11-2v12M9 18a3 3 0 11-3-3 3 3 0 013 3zM20 16a3 3 0 11-3-3 3 3 0 013 3z',
  chat: 'M4 5h16v11H8l-4 3.5z',
  sparkle: 'M12 3l1.8 5.2L19 10l-5.2 1.8L12 17l-1.8-5.2L5 10l5.2-1.8z',
  verified: 'M12 3l2.1 1.5 2.6-.2 1 2.4 2.3 1.2-.6 2.5.9 2.4-1.9 1.7.1 2.6-2.5.6-1.4 2.2H12l-2.6.0-1.4-2.2-2.5-.6.1-2.6L3.7 14l.9-2.4-.6-2.5L6.3 7l1-2.4 2.6.2zM9 12l2 2 4-4',
  link: 'M9.5 14.5l5-5M8 12l-2 2a2.8 2.8 0 104 4l2-2M16 12l2-2a2.8 2.8 0 10-4-4l-2 2',
  'dots-grid': 'M6 6h.01M12 6h.01M18 6h.01M6 12h.01M12 12h.01M18 12h.01M6 18h.01M12 18h.01M18 18h.01',
  history: 'M3 12a9 9 0 109-9 9 9 0 00-7.5 4M3 4v3.5h3.5M12 8v4l3 2',
  star: 'M12 4l2.3 5 5.4.5-4.1 3.6 1.2 5.3-4.8-2.8-4.8 2.8 1.2-5.3L4.3 9.5 9.7 9z',
  back: 'M19 12H5M11 6l-6 6 6 6',
  layers: 'M12 3l9 5-9 5-9-5 9-5zM3 13l9 5 9-5M3 17l9 5 9-5',
  finger: 'M12 11v3a4 4 0 01-4 4M8 11a4 4 0 018 0v2M5 11a7 7 0 0114 0v1M12 18v2',
  bolt: 'M13 2L4 14h7l-1 8 9-12h-7l1-8z',
  doc: 'M6 2h8l4 4v16H6zM14 2v4h4',
  pencil: 'M4 20l4-1L19 8l-3-3L5 16l-1 4zM14 6l3 3',
  chevron: 'M9 6l6 6-6 6',
  chevdown: 'M6 9l6 6 6-6',
};

export type IconName = keyof typeof P;

interface IconProps {
  name: IconName;
  size?: number;
  stroke?: number;
  fill?: string;
  style?: CSSProperties;
  className?: string;
}

export function Icon({ name, size = 20, stroke = 1.7, fill, style, className }: IconProps): JSX.Element {
  const d = P[name];
  if (!d) return <span style={{ width: size, height: size, display: 'inline-block' }} />;
  return (
    <svg
      className={'ic ' + (className || '')}
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke={fill ? 'none' : 'currentColor'}
      strokeWidth={stroke}
      strokeLinecap="round"
      strokeLinejoin="round"
      style={style}
      aria-hidden="true"
    >
      <path d={d} fill={fill || 'none'} />
    </svg>
  );
}

export function CottonMark({ size = 26, color }: { size?: number; color?: string }): JSX.Element {
  const c = color || 'currentColor';
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" aria-hidden="true">
      <g fill={c}>
        <circle cx={12} cy={7.2} r={4.2} />
        <circle cx={16.8} cy={12} r={4.2} />
        <circle cx={12} cy={16.8} r={4.2} />
        <circle cx={7.2} cy={12} r={4.2} />
        <circle cx={12} cy={12} r={3.4} fill="var(--bg)" />
        <circle cx={12} cy={12} r={1.7} />
      </g>
    </svg>
  );
}

export function Brand({ name, size = 20 }: { name: string; size?: number }): JSX.Element | null {
  const common = { width: size, height: size, viewBox: '0 0 24 24' as const, 'aria-hidden': true as const };
  if (name === 'google') {
    return (
      <svg {...common}>
        <path fill="#EA4335" d="M12 10.2v3.9h5.5c-.24 1.4-1 2.6-2.1 3.4v2.8h3.4c2-1.85 3.1-4.56 3.1-7.79 0-.74-.07-1.45-.2-2.13z" />
        <path fill="#4285F4" d="M12 10.2v3.9h5.5c-.24 1.4-1 2.6-2.1 3.4l3.4 2.8c2-1.85 3.2-4.56 3.2-7.89 0-.74-.07-1.45-.2-2.21z" />
        <path fill="#34A853" d="M5.3 14.3l-.8.6-2.7 2.1C3.5 19.9 7.5 22 12 22c2.7 0 5-.9 6.7-2.4l-3.4-2.8c-.9.6-2.1 1-3.3 1-2.6 0-4.8-1.74-5.6-4.08z" />
        <path fill="#FBBC05" d="M2.8 7c-.6 1.2-1 2.6-1 4 0 1.5.4 2.8 1 4l3.5-2.7c-.2-.6-.3-1.2-.3-1.9s.1-1.3.3-1.9z" />
        <path fill="#EA4335" d="M12 5.9c1.5 0 2.8.5 3.8 1.5l2.9-2.9C16.96 2.9 14.7 2 12 2 7.5 2 3.5 4.1 1.8 7.4L5.3 10C6.1 7.7 8.3 5.9 12 5.9z" />
      </svg>
    );
  }
  if (name === 'apple') {
    return (
      <svg {...common}>
        <path fill="currentColor" d="M16.4 12.6c0-2 1.6-3 1.7-3.05-0.93-1.36-2.38-1.55-2.9-1.57-1.23-.13-2.4.72-3.02.72-.63 0-1.59-.7-2.62-.68-1.35.02-2.6.78-3.29 1.99-1.4 2.43-.36 6.03 1 8 .66.96 1.45 2.04 2.49 2 1-.04 1.38-.65 2.59-.65 1.2 0 1.55.65 2.61.63 1.08-.02 1.76-.98 2.42-1.95.76-1.12 1.08-2.2 1.09-2.26-.02-.01-2.09-.8-2.07-3.18zM14.5 6.3c.55-.67.92-1.6.82-2.53-.79.03-1.75.53-2.32 1.2-.51.59-.96 1.53-.84 2.43.88.07 1.79-.45 2.34-1.1z" />
      </svg>
    );
  }
  if (name === 'vk') {
    return (
      <svg {...common}>
        <path fill="#0077FF" d="M12.8 16.4c-5 0-8.2-3.5-8.3-9.2h2.6c.1 4.2 2 6 3.5 6.4V7.2h2.45v3.7c1.5-.16 3-1.84 3.6-3.7h2.4c-.45 2.27-2.1 3.95-3.27 4.65 1.17.57 3.05 2.04 3.77 4.55h-2.7c-.56-1.74-1.95-3.08-3.8-3.27v3.27z" />
      </svg>
    );
  }
  return null;
}
