import { screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Passkeys } from './Passkeys';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import type { Passkey, User } from '../lib/api';
import { renderWithProviders } from '../test/render';

const t = I18N.ru; // RU default.

const FAKE_USER: User = {
  id: 'u1',
  email: 'a@b.c',
  emailVerified: true,
  username: 'alex',
  displayName: 'Alex',
  role: 'user',
};

const FAKE_PASSKEYS: Passkey[] = [
  { id: 'pk-1', name: 'MacBook', createdAt: '2026-01-02T10:00:00Z', lastUsedAt: '2026-02-01T08:00:00Z' },
  { id: 'pk-2', name: 'iPhone', createdAt: '2026-03-04T10:00:00Z' },
];

/** Set or clear `window.PublicKeyCredential` to simulate WebAuthn (un)support. */
function setWebAuthnSupport(supported: boolean): void {
  if (supported) {
    (window as unknown as { PublicKeyCredential: unknown }).PublicKeyCredential = function () {};
  } else {
    delete (window as unknown as { PublicKeyCredential?: unknown }).PublicKeyCredential;
  }
}

describe('Passkeys page', () => {
  beforeEach(() => {
    // A session must resolve for the auth-gate to pass.
    vi.spyOn(apiModule.api, 'session').mockResolvedValue({ user: FAKE_USER });
    // The "Add passkey" action is only offered when WebAuthn is available.
    setWebAuthnSupport(true);
  });
  afterEach(() => {
    vi.restoreAllMocks();
    setWebAuthnSupport(false);
  });

  it('renders the list of the user’s passkeys', async () => {
    vi.spyOn(apiModule.passkeyApi, 'list').mockResolvedValue({ passkeys: FAKE_PASSKEYS });

    renderWithProviders(<Passkeys />, { route: '/passkeys' });

    expect(await screen.findByText('MacBook')).toBeInTheDocument();
    expect(screen.getByText('iPhone')).toBeInTheDocument();
    // Two list items, each with a delete affordance.
    expect(screen.getAllByRole('listitem')).toHaveLength(2);
    expect(screen.getByRole('button', { name: new RegExp(`${t.pk_delete}: MacBook`, 'i') })).toBeInTheDocument();
  });

  it('shows an empty-state message when the user has no passkeys', async () => {
    vi.spyOn(apiModule.passkeyApi, 'list').mockResolvedValue({ passkeys: [] });

    renderWithProviders(<Passkeys />, { route: '/passkeys' });

    expect(await screen.findByText(t.pk_empty)).toBeInTheDocument();
    expect(screen.queryAllByRole('listitem')).toHaveLength(0);
  });

  it('exposes an "Add passkey" action', async () => {
    vi.spyOn(apiModule.passkeyApi, 'list').mockResolvedValue({ passkeys: [] });

    renderWithProviders(<Passkeys />, { route: '/passkeys' });

    await waitFor(() => {
      expect(screen.getByRole('button', { name: new RegExp(t.pk_add, 'i') })).toBeEnabled();
    });
  });
});
