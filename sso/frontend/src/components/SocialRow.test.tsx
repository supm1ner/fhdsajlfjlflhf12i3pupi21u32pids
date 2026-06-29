import { render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { SocialRow } from './SocialRow';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import type { SocialProvider } from '../lib/api';

const t = I18N.en;

/** Stub `getSocialProviders` to resolve with the given enabled-provider list. */
function mockProviders(providers: SocialProvider[]): void {
  vi.spyOn(apiModule, 'getSocialProviders').mockResolvedValue({ providers });
}

describe('SocialRow', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('renders a button for each enabled provider returned by the backend', async () => {
    mockProviders([
      { id: 'google', name: 'Google' },
      { id: 'github', name: 'GitHub' },
      { id: 'vk', name: 'VK' },
      { id: 'yandex', name: 'Yandex' },
    ]);
    render(<SocialRow t={t} />);

    expect(await screen.findByRole('button', { name: /google/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /github/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /vk/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /yandex/i })).toBeInTheDocument();
    // All rendered providers are enabled (no disabled stubs anymore).
    expect(screen.getByRole('button', { name: /google/i })).toBeEnabled();
  });

  it('renders only the providers the backend advertises', async () => {
    mockProviders([{ id: 'google', name: 'Google' }]);
    render(<SocialRow t={t} />);

    expect(await screen.findByRole('button', { name: /google/i })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /github/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /vk/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /yandex/i })).not.toBeInTheDocument();
  });

  it('never renders an Apple button', async () => {
    mockProviders([
      { id: 'google', name: 'Google' },
      { id: 'github', name: 'GitHub' },
    ]);
    render(<SocialRow t={t} />);

    // Wait until the row has rendered, then assert Apple is absent.
    await screen.findByRole('button', { name: /google/i });
    expect(screen.queryByRole('button', { name: /apple/i })).not.toBeInTheDocument();
  });

  it('renders nothing when no providers are enabled', async () => {
    mockProviders([]);
    const { container } = render(<SocialRow t={t} />);

    // It must never render the "or continue with" row when empty.
    await waitFor(() => {
      expect(screen.queryByText(t.or_continue)).not.toBeInTheDocument();
    });
    expect(container).toBeEmptyDOMElement();
  });

  it('renders nothing when the providers fetch fails (graceful degradation)', async () => {
    vi.spyOn(apiModule, 'getSocialProviders').mockRejectedValue(new Error('network'));
    const { container } = render(<SocialRow t={t} />);

    await waitFor(() => {
      expect(container).toBeEmptyDOMElement();
    });
    expect(screen.queryByRole('button', { name: /google/i })).not.toBeInTheDocument();
  });
});
