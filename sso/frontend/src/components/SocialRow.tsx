/* SocialRow.tsx — "or continue with" social provider row (new design) */

import { useEffect, useState } from 'react';
import type { Dict } from '../i18n/dictionary';
import {
  getSocialProviders,
  socialStartUrl,
  type SocialProvider,
  type SocialProviderId,
} from '../lib/api';

const BRANDS: Record<SocialProviderId, { color: string; mark: JSX.Element }> = {
  google: {
    color: '#ffffff',
    mark: (
      <svg width="20" height="20" viewBox="0 0 48 48" aria-hidden="true">
        <path fill="#4285f4" d="M45.12 24.5c0-1.56-.14-3.06-.4-4.5H24v8.51h11.84c-.51 2.75-2.06 5.08-4.39 6.64v5.52h7.11c4.16-3.83 6.56-9.47 6.56-16.17z" />
        <path fill="#34a853" d="M24 46c5.94 0 10.92-1.97 14.56-5.33l-7.11-5.52c-1.97 1.32-4.49 2.1-7.45 2.1-5.73 0-10.58-3.87-12.31-9.07H4.34v5.7C7.96 41.07 15.4 46 24 46z" />
        <path fill="#fbbc05" d="M11.69 28.18c-.44-1.32-.69-2.73-.69-4.18s.25-2.86.69-4.18v-5.7H4.34A21.99 21.99 0 0 0 2 24c0 3.55.85 6.91 2.34 9.88l7.35-5.7z" />
        <path fill="#ea4335" d="M24 10.75c3.23 0 6.13 1.11 8.41 3.29l6.31-6.31C34.91 4.18 29.93 2 24 2 15.4 2 7.96 6.93 4.34 14.12l7.35 5.7c1.73-5.2 6.58-9.07 12.31-9.07z" />
      </svg>
    ),
  },
  github: {
    color: '#1f2328',
    mark: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <path d="M12 .5a12 12 0 0 0-3.79 23.4c.6.1.82-.26.82-.58v-2.2c-3.34.72-4.04-1.6-4.04-1.6-.55-1.38-1.34-1.75-1.34-1.75-1.09-.74.08-.73.08-.73 1.2.09 1.84 1.24 1.84 1.24 1.07 1.84 2.8 1.3 3.49 1 .1-.78.42-1.31.76-1.61-2.67-.3-5.47-1.34-5.47-5.96 0-1.32.47-2.39 1.24-3.23-.13-.3-.54-1.52.12-3.18 0 0 1-.32 3.3 1.23a11.5 11.5 0 0 1 6 0c2.3-1.55 3.3-1.23 3.3-1.23.66 1.66.25 2.88.12 3.18.77.84 1.23 1.91 1.23 3.23 0 4.63-2.8 5.65-5.48 5.95.43.37.81 1.1.81 2.22v3.29c0 .32.22.69.83.57A12 12 0 0 0 12 .5z" />
      </svg>
    ),
  },
  vk: {
    color: '#0077ff',
    mark: (
      <svg width="20" height="20" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
        <path d="M13.16 17.74c-5.47 0-8.59-3.75-8.72-9.99h2.74c.09 4.58 2.11 6.52 3.71 6.92V7.75h2.58v3.95c1.58-.17 3.24-1.97 3.8-3.95h2.58a7.66 7.66 0 0 1-3.5 4.99 7.94 7.94 0 0 1 4.1 4.99h-2.84c-.5-1.56-2.02-2.77-4.14-2.97v2.97h-.31z" />
      </svg>
    ),
  },
  yandex: {
    color: '#fc3f1d',
    mark: (
      <svg width="20" height="20" viewBox="0 0 24 24" aria-hidden="true">
        <circle cx="12" cy="12" r="12" fill="currentColor" />
        <path fill="#ffffff" d="M13.1 6.6h-1.2c-1.9 0-3 1-3 2.9 0 1.6.7 2.4 2.1 3.3L9 18h1.8l1.9-4.6h.9V18h1.6V6.6h-2.1zm-.2 5.5h-.6c-.9 0-1.6-.5-1.6-2 0-1.4.6-2 1.6-2h.6v4z" />
      </svg>
    ),
  },
};

export function SocialRow({ t }: { t: Dict }): JSX.Element | null {
  const [providers, setProviders] = useState<SocialProvider[]>([]);

  useEffect(() => {
    const controller = new AbortController();
    getSocialProviders(controller.signal)
      .then((res) => {
        setProviders((res.providers ?? []).filter((p) => p.id in BRANDS));
      })
      .catch(() => {
        setProviders([]);
      });
    return () => controller.abort();
  }, []);

  if (providers.length === 0) return null;

  const start = (provider: SocialProviderId): void => {
    window.location.assign(socialStartUrl(provider));
  };

  return (
    <div className="col" style={{ gap: 16 }}>
      <div className="row" style={{ gap: 14, color: 'var(--text-3)', fontSize: 13 }}>
        <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
        {t.or_continue}
        <div style={{ flex: 1, height: 1, background: 'var(--border)' }} />
      </div>
      <div className="row" style={{ gap: 10, flexWrap: 'wrap' }}>
        {providers.map((p) => {
          const brand = BRANDS[p.id];
          return (
            <button
              key={p.id}
              type="button"
              title={p.name}
              onClick={() => start(p.id)}
              className="row gap-2"
              style={{
                flex: '1 1 120px',
                justifyContent: 'center',
                height: 44,
                borderRadius: 'var(--r-sm)',
                background: 'var(--surface-2)',
                border: '1px solid var(--border)',
                color: 'var(--text)',
                fontWeight: 540,
                fontSize: 14,
                cursor: 'pointer',
                transition: 'background 0.18s var(--ease)',
              }}
              onMouseEnter={(e) => { e.currentTarget.style.background = 'var(--surface-hover)'; }}
              onMouseLeave={(e) => { e.currentTarget.style.background = 'var(--surface-2)'; }}
            >
              <span style={{ width: 20, height: 20, flexShrink: 0, display: 'inline-grid', placeItems: 'center', color: brand.color }}>
                {brand.mark}
              </span>
              {p.name}
            </button>
          );
        })}
      </div>
    </div>
  );
}
