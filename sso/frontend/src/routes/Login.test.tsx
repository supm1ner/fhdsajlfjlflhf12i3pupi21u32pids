import { screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Login } from './Login';
import { I18N } from '../i18n/dictionary';
import * as apiModule from '../lib/api';
import { renderWithProviders } from '../test/render';

const t = I18N.ru; // RU is the default locale.

/** Set or clear `window.PublicKeyCredential` to simulate WebAuthn (un)support. */
function setWebAuthnSupport(supported: boolean): void {
  if (supported) {
    // A stand-in constructor is enough — the component only checks for presence.
    (window as unknown as { PublicKeyCredential: unknown }).PublicKeyCredential = function () {};
  } else {
    delete (window as unknown as { PublicKeyCredential?: unknown }).PublicKeyCredential;
  }
}

describe('Login passkey button', () => {
  beforeEach(() => {
    // Social row fetches providers on mount — stub it so Login renders cleanly.
    vi.spyOn(apiModule, 'getSocialProviders').mockResolvedValue({ providers: [] });
  });
  afterEach(() => {
    vi.restoreAllMocks();
    setWebAuthnSupport(false);
  });

  it('is hidden when window.PublicKeyCredential is undefined', async () => {
    setWebAuthnSupport(false);
    renderWithProviders(<Login />, { route: '/login' });

    // Flush the SocialRow providers fetch (resolves post-render) inside act.
    await screen.findByRole('button', { name: new RegExp(t.btn_login, 'i') });
    expect(screen.queryByRole('button', { name: new RegExp(t.passkey, 'i') })).toBeNull();
  });

  it('renders an enabled passkey button when WebAuthn is supported', async () => {
    setWebAuthnSupport(true);
    renderWithProviders(<Login />, { route: '/login' });

    const btn = await screen.findByRole('button', { name: new RegExp(t.passkey, 'i') });
    expect(btn).toBeInTheDocument();
    expect(btn).toBeEnabled();
  });

  it('starts the passkey ceremony when clicked', async () => {
    setWebAuthnSupport(true);
    const loginBegin = vi
      .spyOn(apiModule.passkeyApi, 'loginBegin')
      // Reject after begin so we do not need to mock navigator.credentials.get.
      .mockRejectedValue(new apiModule.ApiError(500, { title: 'boom', detail: 'boom' }));

    renderWithProviders(<Login />, { route: '/login' });
    const btn = screen.getByRole('button', { name: new RegExp(t.passkey, 'i') });
    await userEvent.click(btn);

    await waitFor(() => expect(loginBegin).toHaveBeenCalledTimes(1));
    // No email typed and no login_challenge in the URL → discoverable ceremony.
    expect(loginBegin).toHaveBeenCalledWith(undefined, undefined);
    // The begin failure surfaces as an error notice (login error fallback).
    expect(await screen.findByRole('alert')).toBeInTheDocument();
  });
});
